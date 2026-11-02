package inquiry

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"time"
)

// crockford 是 ULID 使用的 base32 字母表（去掉了容易混淆的 I L O U）。
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID 生成一个 ULID：48 bit 毫秒时间戳 + 80 bit 随机数，共 26 字符。
//
// 为什么不用 UUID：询盘 ID 会出现在邮件标题、导出文件和客服沟通里。
// ULID 按时间有序（主键插入不会随机跳跃，索引更友好），前 10 位就是时间，
// 肉眼能看出先后——客户说「我昨天发过」时，翻 ID 就能确认。
func NewID(t time.Time) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	// 前 6 字节（48 bit）放毫秒时间戳
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	// 后 10 字节随机
	if _, err := rand.Read(b[6:]); err != nil {
		// 随机源不可用时退化为纳秒时间戳：宁可 ID 弱一点，也不让询盘写不进来
		binary.BigEndian.PutUint64(b[8:], uint64(t.UnixNano()))
	}
	return encodeULID(b)
}

// encodeULID 把 128 bit 编码成 26 位 Crockford Base32。
// 26×5=130 bit，比 128 bit 多 2 bit，所以最高位自然补 0。
func encodeULID(b [16]byte) string {
	n := new(big.Int).SetBytes(b[:])
	mask := big.NewInt(0x1F)
	tmp := new(big.Int)
	out := make([]byte, 26)
	for i := 25; i >= 0; i-- {
		tmp.And(n, mask)
		out[i] = crockford[tmp.Int64()]
		n.Rsh(n, 5)
	}
	return string(out)
}
