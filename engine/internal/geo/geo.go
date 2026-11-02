// Package geo 提供询盘表单用到的国家/地区清单。
//
// 为什么不引第三方国家库：询盘表单只需要一份「常见出口目的地」的下拉，
// 30 条足够覆盖 95% 的 B2B 询盘；引一个几百 KB 的数据包只为渲染下拉框不划算。
// 需要更全时按 markets 扩展即可。
package geo

// Country 是一个下拉选项。Code 用 ISO 3166-1 alpha-2，入库也只存这个。
type Country struct {
	Code string
	EN   string
	ZH   string
}

// All 返回按英文字母序的常见出口目的地。
var All = []Country{
	{"AU", "Australia", "澳大利亚"},
	{"BR", "Brazil", "巴西"},
	{"CA", "Canada", "加拿大"},
	{"CL", "Chile", "智利"},
	{"DE", "Germany", "德国"},
	{"EG", "Egypt", "埃及"},
	{"ES", "Spain", "西班牙"},
	{"FR", "France", "法国"},
	{"GB", "United Kingdom", "英国"},
	{"ID", "Indonesia", "印度尼西亚"},
	{"IN", "India", "印度"},
	{"IT", "Italy", "意大利"},
	{"JP", "Japan", "日本"},
	{"KR", "South Korea", "韩国"},
	{"MX", "Mexico", "墨西哥"},
	{"MY", "Malaysia", "马来西亚"},
	{"NG", "Nigeria", "尼日利亚"},
	{"NL", "Netherlands", "荷兰"},
	{"PL", "Poland", "波兰"},
	{"RU", "Russia", "俄罗斯"},
	{"SA", "Saudi Arabia", "沙特阿拉伯"},
	{"SE", "Sweden", "瑞典"},
	{"SG", "Singapore", "新加坡"},
	{"TH", "Thailand", "泰国"},
	{"TR", "Türkiye", "土耳其"},
	{"AE", "United Arab Emirates", "阿联酋"},
	{"US", "United States", "美国"},
	{"VN", "Vietnam", "越南"},
	{"ZA", "South Africa", "南非"},
	{"OT", "Other / Not listed", "其他 / 未列出"},
}

// byCode 用于校验，含 "OT" 这个约定值。
var byCode = func() map[string]Country {
	m := make(map[string]Country, len(All))
	for _, c := range All {
		m[c.Code] = c
	}
	return m
}()

// Valid 判断国家代码是否在清单内。
func Valid(code string) bool {
	_, ok := byCode[code]
	return ok
}

// Name 返回指定语言下的国家名，未命中时返回代码本身。
func Name(code, lang string) string {
	c, ok := byCode[code]
	if !ok {
		return code
	}
	if lang == "zh" {
		return c.ZH
	}
	return c.EN
}
