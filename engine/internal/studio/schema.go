// Package studio 是 Web Studio：表单化内容编辑 + 多用户 + 审核流。
//
// 它建在 contentdb（M3 第一阶段的内容库）之上，边界刻意清晰：
//
//   - 只做**表单编辑**，不做 YAML 文本编辑——那是 engine admin 的地盘。
//     表单面向客户（改文案、改状态），admin 面向开发者（改结构）。
//   - 只编辑**内容库**，不碰文件。文件 → 库走 engine import，
//     库 → 文件走 engine export；构建用 engine build --db。
//   - 审核流用状态机表达：draft →(submit)→ in_review →(publish)→ published，
//     publish 与 unpublish 只有 admin 能做——编辑不能自己审自己。
package studio

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/model"
)

// Field 描述一个表单字段。Kind 决定控件，i18n 前缀表示按语言展开。
type Field struct {
	Name  string // 表单字段名；i18n 字段按 name.<lang> 提交
	Label string
	Kind  string // text / textarea / num / bool / list / i18n_text / i18n_textarea
	Help  string
}

// IsI18n 判断是否多语言字段（每语言一个输入框）。
func (f Field) IsI18n() bool { return f.Kind == "i18n_text" || f.Kind == "i18n_textarea" }

// Status 是审核流的状态。published 与引擎原有语义兼容：
// 构建只产出 status == published 的实体，draft / in_review 自动不出现在站上。
const (
	StatusDraft     = "draft"
	StatusInReview  = "in_review"
	StatusPublished = "published"
)

// statusLabel 把状态翻译成界面文案。
var statusLabel = map[string]string{
	StatusDraft:     "草稿",
	StatusInReview:  "待审核",
	StatusPublished: "已发布",
}

// Schema 是一种实体的表单定义。字段列表同时驱动**渲染**与**写入**，
// 两边永远一致——这是表单化编辑最容易烂掉的地方。
type Schema struct {
	Title  string
	Fields []Field
}

var schemas = map[string]Schema{
	"products": {
		Title: "产品",
		Fields: []Field{
			{Name: "title", Label: "产品名", Kind: "i18n_text"},
			{Name: "subtitle", Label: "副标题", Kind: "i18n_text"},
			{Name: "summary", Label: "摘要", Kind: "i18n_textarea"},
			{Name: "description", Label: "详情描述", Kind: "i18n_textarea"},
			{Name: "category", Label: "品类 slug", Kind: "text"},
			{Name: "scenarios", Label: "应用场景 slug（每行一个）", Kind: "list"},
			{Name: "model_no", Label: "型号", Kind: "text"},
			{Name: "moq", Label: "最小起订量", Kind: "num"},
			{Name: "moq_unit", Label: "起订单位", Kind: "text", Help: "如 kg / pcs"},
			{Name: "lead_time", Label: "交期", Kind: "i18n_text"},
			{Name: "packaging", Label: "包装", Kind: "i18n_text"},
			{Name: "hs_code", Label: "HS 编码", Kind: "text"},
			{Name: "cover", Label: "封面图路径", Kind: "text", Help: "如 media/tea.png"},
			{Name: "featured", Label: "首页推荐", Kind: "bool"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
	"pages": {
		Title: "页面",
		Fields: []Field{
			{Name: "title", Label: "页面标题", Kind: "i18n_text"},
			{Name: "summary", Label: "页面摘要", Kind: "i18n_textarea"},
			{Name: "cover", Label: "封面图路径", Kind: "text"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
	"cases": {
		Title: "客户案例",
		Fields: []Field{
			{Name: "title", Label: "案例标题", Kind: "i18n_text"},
			{Name: "country", Label: "国家 / 地区", Kind: "text"},
			{Name: "industry", Label: "行业", Kind: "i18n_text"},
			{Name: "customer", Label: "客户名", Kind: "text"},
			{Name: "challenge", Label: "挑战", Kind: "i18n_textarea"},
			{Name: "solution", Label: "方案", Kind: "i18n_textarea"},
			{Name: "result", Label: "成果", Kind: "i18n_textarea"},
			{Name: "cover", Label: "封面图路径", Kind: "text"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
	"categories": {
		Title: "品类",
		Fields: []Field{
			{Name: "name", Label: "品类名", Kind: "i18n_text"},
			{Name: "desc", Label: "描述", Kind: "i18n_textarea"},
			{Name: "parent", Label: "父品类 slug", Kind: "text"},
			{Name: "cover", Label: "封面图路径", Kind: "text"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
	"scenarios": {
		Title: "应用场景",
		Fields: []Field{
			{Name: "name", Label: "场景名", Kind: "i18n_text"},
			{Name: "summary", Label: "摘要", Kind: "i18n_textarea"},
			{Name: "icon", Label: "图标名", Kind: "text"},
			{Name: "cover", Label: "封面图路径", Kind: "text"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
	"certificates": {
		Title: "证书",
		Fields: []Field{
			{Name: "name", Label: "证书名", Kind: "i18n_text"},
			{Name: "issuer", Label: "颁发机构", Kind: "text"},
			{Name: "number", Label: "证书编号", Kind: "text"},
			{Name: "issued", Label: "签发日期", Kind: "text", Help: "YYYY-MM-DD"},
			{Name: "expires", Label: "有效期至", Kind: "text", Help: "过期的证书前台自动隐藏"},
			{Name: "markets", Label: "适用市场（每行一个）", Kind: "list"},
			{Name: "file", Label: "证书文件路径", Kind: "text"},
			{Name: "logo", Label: "机构 Logo 路径", Kind: "text"},
			{Name: "order", Label: "排序值", Kind: "num"},
		},
	},
}

// SchemaOf 返回某实体的表单定义。
func SchemaOf(table string) (Schema, bool) {
	s, ok := schemas[table]
	return s, ok
}

// applyForm 把表单值写进 payload：按 schema 逐字段应用到模型 struct，
// 再序列化回 YAML。schema 之外的字段（specs / blocks / seo…）因为
// 是先解码进 struct 再整体序列化，**原样保留**——表单不需要认识它们。
func applyForm(table string, payload []byte, form url.Values, langs []string) ([]byte, error) {
	switch table {
	case "products":
		var p model.Product
		if err := yaml.Unmarshal(payload, &p); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := productSetter(&p, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		p.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(p)
	case "pages":
		var p model.Page
		if err := yaml.Unmarshal(payload, &p); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := pageSetter(&p, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		p.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(p)
	case "cases":
		var c model.Case
		if err := yaml.Unmarshal(payload, &c); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := caseSetter(&c, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		c.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(c)
	case "categories":
		var c model.Category
		if err := yaml.Unmarshal(payload, &c); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := categorySetter(&c, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		c.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(c)
	case "scenarios":
		var s model.Scenario
		if err := yaml.Unmarshal(payload, &s); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := scenarioSetter(&s, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		s.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(s)
	case "certificates":
		var c model.Certificate
		if err := yaml.Unmarshal(payload, &c); err != nil {
			return nil, err
		}
		for _, f := range schemas[table].Fields {
			if err := certificateSetter(&c, f, form, langs); err != nil {
				return nil, fmt.Errorf("字段 %s: %w", f.Name, err)
			}
		}
		c.Base.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return yaml.Marshal(c)
	default:
		return nil, fmt.Errorf("未知的实体表：%s", table)
	}
}

// setI18n 把多语言表单值（name.<lang>）写入 I18n 映射。
// 留空 = 删除该语言条目（清空是合法操作）。
func setI18n(dst *model.I18n, name string, form url.Values, langs []string) {
	if *dst == nil {
		*dst = model.I18n{}
	}
	for _, lang := range langs {
		v := strings.TrimSpace(form.Get(name + "." + lang))
		if v == "" {
			delete(*dst, lang)
		} else {
			(*dst)[lang] = v
		}
	}
}

func setList(dst *[]string, name string, form url.Values) {
	*dst = nil
	for _, line := range strings.Split(form.Get(name), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			*dst = append(*dst, v)
		}
	}
}

func setNum(dst *int, name string, form url.Values) error {
	v := strings.TrimSpace(form.Get(name))
	if v == "" {
		*dst = 0
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("应为整数，得到 %q", v)
	}
	*dst = n
	return nil
}

func setFloat(dst *float64, name string, form url.Values) error {
	v := strings.TrimSpace(form.Get(name))
	if v == "" {
		*dst = 0
		return nil
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fmt.Errorf("应为数字，得到 %q", v)
	}
	*dst = n
	return nil
}

func setText(dst *string, name string, form url.Values) {
	*dst = strings.TrimSpace(form.Get(name))
}

func setBool(dst *bool, name string, form url.Values) {
	*dst = form.Get(name) == "on"
}

// 每种实体的字段写入函数。schema 里的每个 Name 都必须有一个分支；
// 新增字段时改 schema + 对应 setter，编译器和测试会夹住两边。

func productSetter(p *model.Product, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "title":
		setI18n(&p.Title, "title", form, langs)
	case "subtitle":
		setI18n(&p.Subtitle, "subtitle", form, langs)
	case "summary":
		setI18n(&p.Summary, "summary", form, langs)
	case "description":
		setI18n(&p.Description, "description", form, langs)
	case "lead_time":
		setI18n(&p.LeadTime, "lead_time", form, langs)
	case "packaging":
		setI18n(&p.Packaging, "packaging", form, langs)
	case "category":
		setText(&p.Category, "category", form)
	case "model_no":
		setText(&p.ModelNo, "model_no", form)
	case "moq_unit":
		setText(&p.MoqUnit, "moq_unit", form)
	case "hs_code":
		setText(&p.HSCode, "hs_code", form)
	case "cover":
		setText(&p.Cover, "cover", form)
	case "scenarios":
		setList(&p.Scenarios, "scenarios", form)
	case "featured":
		setBool(&p.Featured, "featured", form)
	case "moq":
		return setFloat(&p.Moq, "moq", form)
	case "order":
		return setNum(&p.Order, "order", form)
	default:
		return fmt.Errorf("产品表单未实现的字段")
	}
	return nil
}

func pageSetter(p *model.Page, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "title":
		setI18n(&p.Title, "title", form, langs)
	case "summary":
		setI18n(&p.Summary, "summary", form, langs)
	case "cover":
		setText(&p.Cover, "cover", form)
	case "order":
		return setNum(&p.Order, "order", form)
	default:
		return fmt.Errorf("页面表单未实现的字段")
	}
	return nil
}

func caseSetter(c *model.Case, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "title":
		setI18n(&c.Title, "title", form, langs)
	case "industry":
		setI18n(&c.Industry, "industry", form, langs)
	case "challenge":
		setI18n(&c.Challenge, "challenge", form, langs)
	case "solution":
		setI18n(&c.Solution, "solution", form, langs)
	case "result":
		setI18n(&c.Result, "result", form, langs)
	case "country":
		setText(&c.Country, "country", form)
	case "customer":
		setText(&c.Customer, "customer", form)
	case "cover":
		setText(&c.Cover, "cover", form)
	case "order":
		return setNum(&c.Order, "order", form)
	default:
		return fmt.Errorf("案例表单未实现的字段")
	}
	return nil
}

func categorySetter(c *model.Category, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "name":
		setI18n(&c.Name, "name", form, langs)
	case "desc":
		setI18n(&c.Desc, "desc", form, langs)
	case "parent":
		setText(&c.Parent, "parent", form)
	case "cover":
		setText(&c.Cover, "cover", form)
	case "order":
		return setNum(&c.Order, "order", form)
	default:
		return fmt.Errorf("品类表单未实现的字段")
	}
	return nil
}

func scenarioSetter(s *model.Scenario, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "name":
		setI18n(&s.Name, "name", form, langs)
	case "summary":
		setI18n(&s.Summary, "summary", form, langs)
	case "icon":
		setText(&s.Icon, "icon", form)
	case "cover":
		setText(&s.Cover, "cover", form)
	case "order":
		return setNum(&s.Order, "order", form)
	default:
		return fmt.Errorf("场景表单未实现的字段")
	}
	return nil
}

func certificateSetter(c *model.Certificate, f Field, form url.Values, langs []string) error {
	switch f.Name {
	case "name":
		setI18n(&c.Name, "name", form, langs)
	case "issuer":
		setText(&c.Issuer, "issuer", form)
	case "number":
		setText(&c.Number, "number", form)
	case "issued":
		setText(&c.Issued, "issued", form)
	case "expires":
		setText(&c.Expires, "expires", form)
	case "file":
		setText(&c.File, "file", form)
	case "logo":
		setText(&c.Logo, "logo", form)
	case "markets":
		setList(&c.Markets, "markets", form)
	case "order":
		return setNum(&c.Order, "order", form)
	default:
		return fmt.Errorf("证书表单未实现的字段")
	}
	return nil
}

// applySettingsForm 处理 settings 单行（无 slug，专用路径）。
// 只暴露客户会改的字段；询盘 / 部署 / API 等配置留在 YAML 里给开发者。
func applySettingsForm(payload []byte, form url.Values, langs []string) ([]byte, error) {
	var s model.Settings
	if err := yaml.Unmarshal(payload, &s); err != nil {
		return nil, err
	}
	setText(&s.BaseURL, "base_url", form)
	setText(&s.Brand.Name, "brand_name", form)
	setI18n(&s.Brand.Tagline, "brand_tagline", form, langs)
	setText(&s.Company.LegalName, "legal_name", form)
	setText(&s.Contact.Email, "email", form)
	setText(&s.Contact.Phone, "phone", form)
	setText(&s.Contact.WhatsApp, "whatsapp", form)
	setText(&s.Contact.WeChat, "wechat", form)
	return yaml.Marshal(s)
}
