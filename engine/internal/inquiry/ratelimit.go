package inquiry

import "sync"

// ipLocker 提供按 IP 哈希分片的互斥锁。
//
// 为什么需要它：限流是「读计数 → 判断 → 写入」这样一个复合操作，
// 而这两次数据库访问之间没有任何互斥。100 个并发请求会全部读到
// count = 0，于是全部放行——实测阈值 10 时放过了 18 条（见 B3 §9.5）。
//
// 分 64 片的理由：不同 IP 之间保持完全并行，只有同一 IP 的并发被串行化。
// 真实客户不会在同一秒里提交 100 次，所以这个串行化没有代价。
type ipLocker struct {
	stripes [64]sync.Mutex
}

// lock 按 ipHash 取锁，返回解锁函数。
// 用 FNV-1a 而不是内置哈希：它确定、无分配，且不会因进程重启而改变分片。
func (l *ipLocker) lock(ipHash string) func() {
	const (
		offset = 2166136261
		prime  = 16777619
	)
	var h uint32 = offset
	for i := 0; i < len(ipHash); i++ {
		h ^= uint32(ipHash[i])
		h *= prime
	}
	m := &l.stripes[h%uint32(len(l.stripes))]
	m.Lock()
	return m.Unlock
}
