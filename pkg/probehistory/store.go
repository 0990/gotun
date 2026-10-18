// Package probehistory 保存逐次探测结果，与隧道配置独立管理。
package probehistory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

const Retention = 7 * 24 * time.Hour

// Record 是一次原始探测，完整链路结果额外关联来源入口。
type Record struct {
	ID           int64     `json:"id"`
	TunnelUUID   string    `json:"tunnel_uuid"`
	Time         time.Time `json:"time"`
	Kind         string    `json:"kind"`
	RouteUUID    string    `json:"route_uuid"`
	RouteName    string    `json:"route_name"`
	Status       string    `json:"status"`
	DurationMS   int64     `json:"duration_ms"`
	Connectivity string    `json:"connectivity"`
	Error        string    `json:"error"`
}

type Filter struct {
	TunnelUUID string
	Since      time.Time
	Until      time.Time
	Kind       string
	Status     string
	BeforeID   int64
	Limit      int
}

type Page struct {
	Records    []Record `json:"records"`
	NextBefore int64    `json:"next_before"`
}

type writeRequest struct {
	record *Record
	flush  chan error
}

type Store struct {
	db       *sql.DB
	queue    chan writeRequest
	done     chan struct{}
	stopping chan struct{}
	mu       sync.RWMutex
	closed   bool
	closeErr error
}

func Open(filename string) (*Store, error) {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, err
	}
	// 每个连接均设置等待超时；历史文件仅放在本地磁盘。
	uriPath := filepath.ToSlash(filename)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	params := url.Values{"_pragma": {"busy_timeout(5000)", "synchronous(FULL)"}}
	u.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db, queue: make(chan writeRequest, 4096), done: make(chan struct{}), stopping: make(chan struct{})}
	// 空库必须先设置回收方式，再启用 WAL 创建数据库头。
	_, err = db.Exec(`PRAGMA auto_vacuum=INCREMENTAL;
		PRAGMA journal_mode=WAL;
		CREATE TABLE IF NOT EXISTS probe_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tunnel_uuid TEXT NOT NULL,
			probed_at INTEGER NOT NULL,
			kind TEXT NOT NULL,
			route_uuid TEXT NOT NULL,
			route_name TEXT NOT NULL,
			status TEXT NOT NULL,
			duration_ms INTEGER NOT NULL,
			connectivity TEXT NOT NULL,
			error TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS probe_tunnel_id ON probe_history(tunnel_uuid, id DESC);
		CREATE INDEX IF NOT EXISTS probe_time ON probe_history(probed_at);`)
	if err == nil {
		err = s.prune(time.Now())
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("打开探测历史失败: %w", err)
	}
	go s.run()
	return s, nil
}

// Record 将结果交给独立写入循环，避免磁盘延迟阻塞探测和转发。
// 队列溢出会明确报错，由调用者记录日志。
func (s *Store) Record(record Record) error {
	if record.TunnelUUID == "" {
		return errors.New("探测历史缺少隧道 UUID")
	}
	if record.Time.IsZero() {
		record.Time = time.Now()
	}
	// 限制异常文本长度，避免长期失败时历史文件无限膨胀。
	if len(record.Error) > 2048 {
		record.Error = strings.ToValidUTF8(record.Error[:2048], "")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errors.New("探测历史已关闭")
	}
	select {
	case s.queue <- writeRequest{record: &record}:
		return nil
	default:
		return errors.New("探测历史写入队列已满")
	}
}

// Flush 等待此前入队的结果落盘，用于正常退出及一致性检查。
func (s *Store) Flush(ctx context.Context) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return errors.New("探测历史已关闭")
	}
	result := make(chan error, 1)
	select {
	case s.queue <- writeRequest{flush: result}:
		s.mu.RUnlock()
	case <-ctx.Done():
		s.mu.RUnlock()
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.queue)
		close(s.stopping)
	}
	s.mu.Unlock()
	<-s.done
	return s.closeErr
}

func (s *Store) run() {
	defer close(s.done)
	tick := time.NewTicker(time.Second)
	cleanup := time.NewTicker(time.Hour)
	defer tick.Stop()
	defer cleanup.Stop()
	batch := make([]Record, 0, 256)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := s.insert(batch)
		if err != nil {
			logrus.WithError(err).WithField("count", len(batch)).Error("探测历史批量写入失败")
			return err
		}
		batch = batch[:0]
		return err
	}
	finish := func() {
		var waiters []chan error
		for req := range s.queue {
			if req.record != nil {
				batch = append(batch, *req.record)
			}
			if req.flush != nil {
				waiters = append(waiters, req.flush)
			}
		}
		s.closeErr = errors.Join(flush(), s.db.Close())
		for _, waiter := range waiters {
			waiter <- s.closeErr
		}
	}
	for {
		input := s.queue
		// 写入失败时保留批次并重试，同时限制内存占用。
		if len(batch) >= 4096 {
			input = nil
		}
		select {
		case <-s.stopping:
			finish()
			return
		case req, ok := <-input:
			if !ok {
				finish()
				return
			}
			if req.record != nil {
				batch = append(batch, *req.record)
			}
			if req.flush != nil {
				req.flush <- flush()
			} else if len(batch) >= 256 {
				flush()
			}
		case <-tick.C:
			flush()
		case now := <-cleanup.C:
			if err := s.prune(now); err != nil {
				logrus.WithError(err).Error("清理过期探测历史失败")
			}
		}
	}
}

func (s *Store) insert(records []Record) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO probe_history(tunnel_uuid, probed_at, kind, route_uuid, route_name, status, duration_ms, connectivity, error) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		if _, err := stmt.Exec(r.TunnelUUID, r.Time.UnixMilli(), r.Kind, r.RouteUUID, r.RouteName, r.Status, r.DurationMS, r.Connectivity, r.Error); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) prune(now time.Time) error {
	// 分批删除，避免长事务占用写锁；查询也始终排除 7 天以前的记录。
	for {
		result, err := s.db.Exec(`DELETE FROM probe_history WHERE id IN (SELECT id FROM probe_history WHERE probed_at < ? LIMIT 5000)`, now.Add(-Retention).UnixMilli())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n < 5000 {
			break
		}
	}
	_, err := s.db.Exec("PRAGMA incremental_vacuum(1000)")
	return err
}

func (s *Store) Query(ctx context.Context, f Filter) (Page, error) {
	page := Page{Records: []Record{}}
	if f.TunnelUUID == "" {
		return page, errors.New("缺少隧道 UUID")
	}
	cutoff := time.Now().Add(-Retention)
	if f.Since.Before(cutoff) {
		f.Since = cutoff
	}
	if f.Until.IsZero() {
		f.Until = time.Now()
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	query := `SELECT id, tunnel_uuid, probed_at, kind, route_uuid, route_name, status, duration_ms, connectivity, error FROM probe_history WHERE tunnel_uuid = ? AND probed_at >= ? AND probed_at <= ?`
	args := []any{f.TunnelUUID, f.Since.UnixMilli(), f.Until.UnixMilli()}
	for _, condition := range []struct{ column, value string }{{"kind", f.Kind}, {"status", f.Status}} {
		if condition.value != "" {
			query += " AND " + condition.column + " = ?"
			args = append(args, condition.value)
		}
	}
	if f.BeforeID > 0 {
		query += " AND id < ?"
		args = append(args, f.BeforeID)
	}
	// ID 按落盘顺序递增，游标分页在持续写入时不会重复或跳过已有结果。
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, f.Limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Record
		var ms int64
		if err := rows.Scan(&r.ID, &r.TunnelUUID, &ms, &r.Kind, &r.RouteUUID, &r.RouteName, &r.Status, &r.DurationMS, &r.Connectivity, &r.Error); err != nil {
			return page, err
		}
		r.Time = time.UnixMilli(ms).UTC()
		page.Records = append(page.Records, r)
	}
	if len(page.Records) > f.Limit {
		page.Records = page.Records[:f.Limit]
		page.NextBefore = page.Records[len(page.Records)-1].ID
	}
	return page, rows.Err()
}
