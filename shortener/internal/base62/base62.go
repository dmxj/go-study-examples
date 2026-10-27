// Package base62 实现整数与 base62 字符串的互相转换。
//
// 短链接服务把数据库自增 ID 编码为 base62：62 个字符 [0-9a-zA-Z] 都是 URL 安全的，
// 6 位就能表示 62^6 ≈ 568 亿个链接，比十进制短得多。
package base62

import (
	"errors"
	"strings"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// decodeMap 把字符映射回数值，-1 表示非法字符。在 init 阶段一次性构建。
var decodeMap [256]int8

func init() {
	for i := range decodeMap {
		decodeMap[i] = -1
	}
	for i := 0; i < len(alphabet); i++ {
		decodeMap[alphabet[i]] = int8(i)
	}
}

// ErrInvalid 表示字符串包含非 base62 字符或溢出 uint64。
var ErrInvalid = errors.New("非法的 base62 字符串")

// Encode 把 n 编码为 base62。算法与十进制转二进制相同：反复除以 62 取余数，再倒序。
func Encode(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [11]byte // uint64 最大值的 base62 表示为 11 位
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = alphabet[n%62]
		n /= 62
	}
	return string(buf[i:])
}

// Decode 是 Encode 的逆运算。
func Decode(s string) (uint64, error) {
	if s == "" || len(s) > 11 {
		return 0, ErrInvalid
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		d := decodeMap[s[i]]
		if d < 0 {
			return 0, ErrInvalid
		}
		// 溢出检查：n*62 + d 不能超过 MaxUint64
		if n > (^uint64(0)-uint64(d))/62 {
			return 0, ErrInvalid
		}
		n = n*62 + uint64(d)
	}
	return n, nil
}

// Valid 报告 s 是否只由 base62 字符组成。
func Valid(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return r > 255 || decodeMap[r] < 0 }) < 0
}
