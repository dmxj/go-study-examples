// Package database 负责打开 SQLite 数据库并执行迁移。
package database

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite" // 纯 Go 实现的 SQLite 驱动，无需 cgo，交叉编译和 Docker 构建都更简单
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dmxj/go-study-examples/rest-api/internal/model"
)

// Open 打开数据库、设置连接池并自动迁移表结构。
//
// dsn 可以是文件路径（如 data/books.db）或内存库（如 file::memory:?cache=shared）。
func Open(dsn string, log *slog.Logger) (*gorm.DB, error) {
	if !strings.Contains(dsn, ":memory:") && !strings.HasPrefix(dsn, "file:") {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录: %w", err)
		}
		// WAL 模式提升并发读性能；busy_timeout 让写锁冲突时等待而不是立即报 SQLITE_BUSY。
		dsn += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// TranslateError 让 GORM 把驱动特有的错误翻译成 gorm.ErrDuplicatedKey 等通用错误，
		// repository 就能用 errors.Is 判断唯一键冲突，而不是匹配错误字符串。
		TranslateError: true,
		Logger:         newGormLogger(log),
	})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite 同一时刻只允许一个写者，连接数过多只会增加锁竞争。
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.AutoMigrate(&model.User{}, &model.Book{}); err != nil {
		return nil, fmt.Errorf("迁移表结构: %w", err)
	}
	return db, nil
}

// Close 关闭底层连接池。
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// newGormLogger 让 GORM 的慢查询/错误日志也走 slog，统一日志格式。
func newGormLogger(log *slog.Logger) logger.Interface {
	return logger.NewSlogLogger(log, logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true, // "查不到"是正常业务情况，不打日志
	})
}
