package sfx

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"sync"
)

// hashParts 对若干可序列化部分计算确定性内容哈希。
// JSON 序列化按结构体字段声明顺序输出，结果跨进程稳定。
func hashParts(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		b, err := json.Marshal(p)
		if err != nil {
			panic(err)
		}
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// floatsToBytes 将浮点缓冲编码为小端字节流（确定性）。
func floatsToBytes(src []float64) []byte {
	out := make([]byte, len(src)*8)
	for i, v := range src {
		binary.LittleEndian.PutUint64(out[i*8:], math.Float64bits(v))
	}
	return out
}

// bytesToFloats 解码浮点缓冲；每次调用产生全新切片，调用方互不影响。
func bytesToFloats(b []byte) []float64 {
	out := make([]float64, len(b)/8)
	for i := range out {
		out[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[i*8:]))
	}
	return out
}

// Cache 内容寻址缓存，键为参数内容的哈希，并发安全。
// 相同键必然对应相同内容，因此缓存复用与全量重建逐字节一致。
type Cache struct {
	mu sync.Mutex
	m  map[string][]byte
}

// NewCache 创建内存缓存。
func NewCache() *Cache { return &Cache{m: make(map[string][]byte)} }

// Get 查询缓存。
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	return b, ok
}

// Put 写入缓存。
func (c *Cache) Put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = data
}

// Len 返回缓存条目数（用于测试与诊断）。
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
