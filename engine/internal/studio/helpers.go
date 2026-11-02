package studio

import (
	"io"
	"log/slog"
	"strconv"
)

// 测试辅助（也与生产共用）：丢弃日志、整数格式化。

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func itoa64(n int64) string {
	return strconv.FormatInt(n, 10)
}
