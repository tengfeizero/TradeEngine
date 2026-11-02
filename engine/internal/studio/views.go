package studio

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// 本文件把 payload（YAML 文本）变成模板可以直接渲染的字段视图。
// 读取用 map 探测（只读展示），写入走 schema.go 的模型 struct——
// 两边对同一份 payload，语义由测试夹住。

// FieldView 是一个字段的当前值 + 定义，供模板渲染控件。
type FieldView struct {
	Field
	// 语言顺序（表单按此展开输入框）
	LangOrder []string
	// I18n 字段：每语言的当前值
	LangValues map[string]string
	// 单值字段：当前值（text/textarea/num/list）
	Value string
	// bool 字段
	Checked bool
}

func buildFieldViews(schema Schema, payload string, langs []string) []FieldView {
	var doc map[string]any
	_ = yaml.Unmarshal([]byte(payload), &doc)
	out := make([]FieldView, 0, len(schema.Fields))
	for _, f := range schema.Fields {
		v := FieldView{Field: f, LangOrder: langs}
		switch {
		case f.IsI18n():
			v.LangValues = map[string]string{}
			m, _ := doc[f.Name].(map[string]any)
			for _, lang := range langs {
				if s, ok := m[lang].(string); ok {
					v.LangValues[lang] = s
				}
			}
		case f.Kind == "bool":
			v.Checked, _ = doc[f.Name].(bool)
		case f.Kind == "list":
			if items, ok := doc[f.Name].([]any); ok {
				var lines []string
				for _, it := range items {
					if s, ok := it.(string); ok {
						lines = append(lines, s)
					}
				}
				v.Value = strings.Join(lines, "\n")
			}
		default:
			v.Value = scalarString(doc[f.Name])
		}
		out = append(out, v)
	}
	return out
}

var settingsFields = []Field{
	{Name: "base_url", Label: "站点 URL", Kind: "text", Help: "如 https://www.example.com"},
	{Name: "brand_name", Label: "品牌名", Kind: "text"},
	{Name: "brand_tagline", Label: "品牌标语", Kind: "i18n_text"},
	{Name: "legal_name", Label: "公司注册名", Kind: "text"},
	{Name: "email", Label: "联系邮箱", Kind: "text"},
	{Name: "phone", Label: "电话", Kind: "text"},
	{Name: "whatsapp", Label: "WhatsApp", Kind: "text"},
	{Name: "wechat", Label: "微信", Kind: "text"},
}

func buildSettingsViews(payload string, langs []string) []FieldView {
	var doc map[string]any
	_ = yaml.Unmarshal([]byte(payload), &doc)

	// settings 是嵌套结构：把扁平字段名映射到嵌套路径
	get := func(name string) any {
		switch name {
		case "base_url":
			return doc["base_url"]
		case "brand_name":
			return getMap(doc, "brand")["name"]
		case "brand_tagline":
			return getMap(doc, "brand")["tagline"]
		case "legal_name":
			return getMap(doc, "company")["legal_name"]
		case "email":
			return getMap(doc, "contact")["email"]
		case "phone":
			return getMap(doc, "contact")["phone"]
		case "whatsapp":
			return getMap(doc, "contact")["whatsapp"]
		case "wechat":
			return getMap(doc, "contact")["wechat"]
		}
		return nil
	}
	out := make([]FieldView, 0, len(settingsFields))
	for _, f := range settingsFields {
		v := FieldView{Field: f, LangOrder: langs}
		if f.IsI18n() {
			v.LangValues = map[string]string{}
			m, _ := get(f.Name).(map[string]any)
			for _, lang := range langs {
				if s, ok := m[lang].(string); ok {
					v.LangValues[lang] = s
				}
			}
		} else {
			v.Value = scalarString(get(f.Name))
		}
		out = append(out, v)
	}
	return out
}

func getMap(doc map[string]any, key string) map[string]any {
	if doc == nil {
		return map[string]any{}
	}
	m, _ := doc[key].(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

// scalarString 把 YAML 标量渲染成输入框的字符串值。
func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}
