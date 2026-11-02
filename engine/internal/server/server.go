// Package server 提供运行时服务：静态站 + 唯一的写入口 /api/inquiry。
//
// 对外暴露面刻意只有 3 个（《11》10.1）：
//
//	POST /api/inquiry   唯一写入口
//	GET  /healthz       存活检查
//	GET  /*             静态产物
//
// 每多一个端点，就多一个攻击面和一整套要测的东西。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Options 是服务启动参数。
type Options struct {
	Addr string
	// PublicDir 是静态产物目录（engine build 的 output）。
	PublicDir string
	// InquiryHandler 处理 POST /api/inquiry；为 nil 时不注册该路由。
	InquiryHandler http.Handler
	// AllowedOrigins 是跨源提交白名单（静态站与 API 不同源时必须配置）。
	AllowedOrigins []string
	Version        string
	Log            *slog.Logger
}

// Server 包装 http.Server。
type Server struct {
	opts Options
	log  *slog.Logger
	srv  *http.Server
}

// New 组装路由。返回的 Server 可调用 Run 启动。
func New(opts Options) *Server {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.Addr == "" {
		opts.Addr = ":8080"
	}
	s := &Server{opts: opts, log: opts.Log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	if opts.InquiryHandler != nil {
		mux.Handle("POST /api/inquiry", s.cors(opts.InquiryHandler))
		// 预检请求单独放行：浏览器在跨源 POST 前会先发 OPTIONS
		mux.HandleFunc("OPTIONS /api/inquiry", s.preflight)
	}
	mux.HandleFunc("/", s.static)

	s.srv = &http.Server{
		Addr:    opts.Addr,
		Handler: s.recoverer(s.accessLog(mux)),
		// 超时是防慢速攻击的基本盘：没有它，一个挂着的连接就能占住一个 goroutine
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s
}

// Addr 返回实际监听地址（测试中用 :0 时可从这里读回真实端口）。
func (s *Server) Addr() string { return s.srv.Addr }

// SetAddr 允许测试时改用随机端口。
func (s *Server) SetAddr(a string) { s.srv.Addr = a }

// Handler 暴露内部 handler，便于 httptest 直接调用而不起监听。
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// Run 启动服务并阻塞，直到 ctx 取消后优雅关闭。
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("服务启动", "addr", s.srv.Addr, "public", s.opts.PublicDir)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// 给在途请求 10 秒收尾，避免客户正在提交时被硬断
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.log.Info("正在关闭服务")
		return s.srv.Shutdown(shutdownCtx)
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"version": s.opts.Version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// cors 按白名单放开跨源。不收 "*"：询盘接口带客户隐私，
// 允许任意源提交会让别人能把你的表单当成收集数据的入口。
func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range s.opts.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(r.Header.Get("Origin"), "/")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			// Vary 必须带 Origin，否则 CDN 会把 A 站点的响应缓存给 B 站点
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Origin")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	Static(s.opts.PublicDir).ServeHTTP(w, r)
}

// Static 返回静态产物 handler，供 admin 后台复用同一套缓存与 404 逻辑。
//
// 放在包级而不是 Server 方法上：admin 与 server 的差异只在路由集合，
// 静态文件的行为（目录补 index.html、HTML 不缓存、404 兜底页）必须一致，
// 否则「预览正常、上线异常」这类漂移迟早出现。
func Static(publicDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clean := path.Clean("/" + r.URL.Path)
		// path.Clean 已经消掉了 ..，这里再挡一次编码变体
		if strings.Contains(clean, "..") {
			notFoundPage(w, r, publicDir)
			return
		}

		full := filepath.Join(publicDir, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
		info, err := os.Stat(full)
		if err == nil && info.IsDir() {
			full = filepath.Join(full, "index.html")
			info, err = os.Stat(full)
		}
		if err != nil || info.IsDir() {
			notFoundPage(w, r, publicDir)
			return
		}

		// 三级缓存策略：
		//   HTML —— no-cache，内容更新立刻生效（协商缓存即可，HTML 很小）
		//   带内容哈希的媒体 —— 一年 immutable，文件名变了 URL 才变
		//   其余资源 —— 1 小时（没有哈希兜底，不敢更长）
		if immutableRe.MatchString(full) || strings.Contains(full, ".derived") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(full, ".html") {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		http.ServeFile(w, r, full)
	})
}

func notFoundPage(w http.ResponseWriter, r *http.Request, publicDir string) {
	custom := filepath.Join(publicDir, "404.html")
	if data, err := os.ReadFile(custom); err == nil {
		// 直接 WriteHeader + Write，不走 ServeFile——后者会再写一次状态头
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(data)
		return
	}
	http.Error(w, "404 page not found", http.StatusNotFound)
}

// accessLog 记录每次请求。结构化日志是为了能直接 grep 出「哪个来源页带来的询盘最多」。
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		// 静态资源不记录，否则日志会被图片请求淹掉
		if strings.HasPrefix(r.URL.Path, "/api/") || sw.status >= 400 {
			s.log.Info("http",
				"method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

// recoverer 兜住 panic：一个处理器的 bug 不该让整个服务挂掉。
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("请求处理 panic", "path", r.URL.Path, "panic", rec)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal"}`))
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// immutableRe 匹配构建期写入的内容哈希文件名（name.3fa2b1c9.png）。
var immutableRe = regexp.MustCompile(`\.[0-9a-f]{8}\.[A-Za-z0-9]+$`)

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wrote = true
	return w.ResponseWriter.Write(b)
}
