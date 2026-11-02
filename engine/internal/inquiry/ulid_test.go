package inquiry

import (
	"math/big"
	"strings"
	"testing"
	"time"
)

// decodeTime 从 ULID 前 10 字符还原毫秒时间戳，用于验证编码正确性。
func decodeTime(t *testing.T, id string) int64 {
	t.Helper()
	n := new(big.Int)
	for _, r := range id[:10] {
		idx := strings.IndexRune(crockford, r)
		if idx < 0 {
			t.Fatalf("ULID 含非法字符 %q：%s", r, id)
		}
		n.Mul(n, big.NewInt(32))
		n.Add(n, big.NewInt(int64(idx)))
	}
	return n.Int64()
}

func TestNewIDFormat(t *testing.T) {
	id := NewID(time.Now())

	if len(id) != 26 {
		t.Fatalf("ULID 长度应为 26，实际 %d：%s", len(id), id)
	}
	for _, r := range id {
		if !strings.ContainsRune(crockford, r) {
			t.Fatalf("ULID 含字母表之外的字符 %q：%s", r, id)
		}
	}
}

func TestNewIDEncodesTimestamp(t *testing.T) {
	now := time.Now()
	id := NewID(now)

	got := decodeTime(t, id)
	want := now.UnixMilli()
	if diff := got - want; diff < -2 || diff > 2 {
		t.Fatalf("时间戳还原错误：期望 %d，实际 %d（差 %d ms）", want, got, diff)
	}
}

// ULID 相对 UUID 的核心优势：按时间有序。
// 这条性质让主键插入不随机跳跃（索引友好），也让客服能靠 ID 判断先后。
func TestNewIDIsTimeOrdered(t *testing.T) {
	base := time.Now()
	earlier := NewID(base)
	later := NewID(base.Add(time.Second))

	if earlier >= later {
		t.Fatalf("后生成的时间戳应当更大：%s vs %s", earlier, later)
	}
}

func TestNewIDUniqueness(t *testing.T) {
	now := time.Now()
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		// 同一毫秒内生成，靠 80 bit 随机数区分
		id := NewID(now)
		if seen[id] {
			t.Fatalf("同一毫秒内产生重复 ID：%s", id)
		}
		seen[id] = true
	}
}
