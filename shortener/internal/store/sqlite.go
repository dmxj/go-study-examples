// Package store 负责短链接的持久化（SQLite，database/sql）。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 的 SQLite 驱动，注册名为 "sqlite"；空导入只为执行其 init 注册驱动

	"github.com/dmxj/go-study-examples/shortener/internal/base62"
)

// Link 是一条短链接记录。
type Link struct {
	ID           int64
	Code         string
	URL          string
	Clicks       int64
	CreatedAt    time.Time
	LastAccessAt time.Time // 零值表示从未访问
	ExpiresAt    time.Time // 零值表示永不过期
}

// Expired 报告链接在 now 时刻是否已过期。
func (l Link) Expired(now time.Time) bool {
	return !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt)
}

// 存储层错误。
var (
	ErrNotFound  = errors.New("短链接不存在")
	ErrCodeTaken = errors.New("短码已被占用")
)

const schema = `
CREATE TABLE IF NOT EXISTS links (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	code           TEXT UNIQUE,
	url            TEXT NOT NULL,
	clicks         INTEGER NOT NULL DEFAULT 0,
	created_at     INTEGER NOT NULL,
	last_access_at INTEGER NOT NULL DEFAULT 0,
	expires_at     INTEGER NOT NULL DEFAULT 0
);`

// SQLite 实现存储。时间统一存 Unix 毫秒整数，避免不同驱动对 DATETIME 的解析差异。
type SQLite struct {
	db *sql.DB
}

// Open 打开数据库并建表。path 可以是文件路径或 ":memory:"。
func Open(path string) (*SQLite, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	if path == ":memory:" {
		dsn = "file::memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 只允许一个写者；内存库每个连接都是独立的库，所以必须限制为 1 个连接。
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建表: %w", err)
	}
	return &SQLite{db: db}, nil
}

// Close 关闭数据库。
func (s *SQLite) Close() error { return s.db.Close() }

// Ping 检查数据库连通性。
func (s *SQLite) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Create 新建短链接。code 为空时根据自增 ID 生成 base62 短码。
//
// 生成流程放在一个事务里：先插入拿到自增 ID，再写入编码后的短码。
// 若生成的短码恰好被某个自定义短码占用，就追加后缀重试。
func (s *SQLite) Create(ctx context.Context, url, code string, now, expiresAt time.Time) (*Link, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // Commit 成功后再 Rollback 是无害的空操作

	var codeArg any // nil 会写入 NULL；UNIQUE 列允许多个 NULL
	if code != "" {
		codeArg = code
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO links (code, url, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		codeArg, url, now.UnixMilli(), unixMilli(expiresAt))
	if isUniqueErr(err) {
		return nil, ErrCodeTaken
	}
	if err != nil {
		return nil, fmt.Errorf("插入短链接: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	if code == "" {
		code = base62.Encode(uint64(id))
		for i := 1; ; i++ {
			var taken bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)`, code).Scan(&taken); err != nil {
				return nil, err
			}
			if !taken {
				break
			}
			code = base62.Encode(uint64(id)) + "-" + base62.Encode(uint64(i))
		}
		if _, err := tx.ExecContext(ctx, `UPDATE links SET code = ? WHERE id = ?`, code, id); err != nil {
			return nil, fmt.Errorf("写入短码: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Link{ID: id, Code: code, URL: url, CreatedAt: now, ExpiresAt: expiresAt}, nil
}

// Get 按短码查询。
func (s *SQLite) Get(ctx context.Context, code string) (*Link, error) {
	var (
		l                        Link
		created, access, expires int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, code, url, clicks, created_at, last_access_at, expires_at FROM links WHERE code = ?`, code).
		Scan(&l.ID, &l.Code, &l.URL, &l.Clicks, &created, &access, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	l.CreatedAt = fromMilli(created)
	l.LastAccessAt = fromMilli(access)
	l.ExpiresAt = fromMilli(expires)
	return &l, nil
}

// ClickDelta 是一批待写入的访问统计。
type ClickDelta struct {
	Count      int64
	LastAccess time.Time
}

// AddClicks 在一个事务里批量累加访问次数。
// 使用预编译语句（Prepare）复用 SQL 解析结果，批量更新时更快。
func (s *SQLite) AddClicks(ctx context.Context, deltas map[string]ClickDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx,
		`UPDATE links SET clicks = clicks + ?, last_access_at = MAX(last_access_at, ?) WHERE code = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for code, d := range deltas {
		if _, err := stmt.ExecContext(ctx, d.Count, d.LastAccess.UnixMilli(), code); err != nil {
			return fmt.Errorf("更新 %s 访问统计: %w", code, err)
		}
	}
	return tx.Commit()
}

func isUniqueErr(err error) bool {
	// modernc 驱动的错误类型带有扩展错误码；这里用错误信息判断，避免依赖驱动内部包。
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func fromMilli(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
