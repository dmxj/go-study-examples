package todo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DateLayout 是截止日期的输入/展示格式。
const DateLayout = "2006-01-02"

// ErrInvalidDate 表示无法解析的日期字符串。
var ErrInvalidDate = errors.New("无效的日期")

// ParseDue 解析截止日期，支持以下写法（均相对 now 计算）：
//
//	2026-11-01   绝对日期
//	today        今天
//	tomorrow     明天
//	+3d          3 天后
//	+2w          2 周后
//
// 返回值为当天 00:00（now 的时区）。空字符串返回零值，表示"不设置"。
func ParseDue(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	today := startOfDay(now)
	switch s {
	case "":
		return time.Time{}, nil
	case "today", "今天":
		return today, nil
	case "tomorrow", "明天":
		return today.AddDate(0, 0, 1), nil
	}
	if strings.HasPrefix(s, "+") && len(s) >= 3 {
		n, err := strconv.Atoi(s[1 : len(s)-1])
		if err != nil || n < 0 {
			return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDate, s)
		}
		switch s[len(s)-1] {
		case 'd':
			return today.AddDate(0, 0, n), nil
		case 'w':
			return today.AddDate(0, 0, 7*n), nil
		}
		return time.Time{}, fmt.Errorf("%w: %q（相对日期只支持 d/w 单位）", ErrInvalidDate, s)
	}
	// time.ParseInLocation 让日期落在 now 所在的时区，避免默认 UTC 导致"差一天"。
	t, err := time.ParseInLocation(DateLayout, s, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q（格式 YYYY-MM-DD / today / tomorrow / +3d）", ErrInvalidDate, s)
	}
	return t, nil
}
