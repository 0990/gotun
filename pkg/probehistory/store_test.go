package probehistory

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPersistenceRetentionAndCursor(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "带空格 history", "probe-history.db")
	s, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	var vacuum int
	if err := s.db.QueryRow("PRAGMA auto_vacuum").Scan(&vacuum); err != nil || vacuum != 2 {
		s.Close()
		t.Fatalf("历史库应启用增量空间回收：%d %v", vacuum, err)
	}
	now := time.Now().Add(-time.Second).Truncate(time.Millisecond)
	for i := 0; i < 125; i++ {
		r := Record{TunnelUUID: "tunnel-a", Time: now, Kind: "frame", Status: "normal", DurationMS: int64(i)}
		if i%2 == 0 {
			r.Kind, r.Status, r.RouteUUID, r.RouteName, r.Error = "route", "failed", "route-a", "入口甲", "超时"
		}
		if err := s.Record(r); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range []Record{
		{TunnelUUID: "tunnel-a", Time: now.Add(-Retention), Kind: "frame", Status: "failed"},
		{TunnelUUID: "tunnel-b", Time: now, Kind: "frame", Status: "failed"},
	} {
		if err := s.Record(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(Record{TunnelUUID: "tunnel-a"}); err == nil {
		t.Fatal("关闭后应拒绝写入")
	}
	s, err = Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM probe_history").Scan(&count); err != nil || count != 126 {
		t.Fatalf("启动应物理清理过期记录，保留 126 条：%d %v", count, err)
	}
	seen := map[int64]bool{}
	f := Filter{TunnelUUID: "tunnel-a", Limit: 50, Until: now.Add(time.Millisecond)}
	for {
		page, err := s.Query(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Records {
			if seen[r.ID] || r.TunnelUUID != "tunnel-a" || !r.Time.Equal(now) {
				t.Fatalf("重复、串隧道或时间有误：%+v", r)
			}
			seen[r.ID] = true
		}
		if f.BeforeID == 0 {
			// 同一时间新入库的记录不能干扰已开始的翻页。
			if err := s.Record(Record{TunnelUUID: "tunnel-a", Time: now, Kind: "frame", Status: "normal"}); err != nil {
				t.Fatal(err)
			}
			if err := s.Flush(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		if page.NextBefore == 0 {
			break
		}
		f.BeforeID = page.NextBefore
	}
	if len(seen) != 125 {
		t.Fatalf("分页应完整返回原先 125 条：%d", len(seen))
	}
	page, err := s.Query(context.Background(), Filter{TunnelUUID: "tunnel-a", Kind: "route", Status: "failed", Limit: 100})
	if err != nil || len(page.Records) != 63 || page.Records[0].RouteName != "入口甲" || page.Records[0].Error != "超时" {
		t.Fatalf("类型与结果筛选失败：%+v %v", page, err)
	}
	if err := s.prune(now.Add(Retention + time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM probe_history").Scan(&count); err != nil || count != 0 {
		t.Fatalf("定期清理应删除全部过期结果：%d %v", count, err)
	}
}

func TestConcurrentRecordingAndQuery(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 150; j++ {
				if err := s.Record(Record{TunnelUUID: fmt.Sprint(i), Kind: "frame", Status: "normal"}); err != nil {
					t.Error(err)
				}
				if _, err := s.Query(context.Background(), Filter{TunnelUUID: fmt.Sprint(i)}); err != nil {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM probe_history").Scan(&count); err != nil || count != 600 {
		t.Fatalf("并发写入不得丢失：%d %v", count, err)
	}
}

func TestFailedBatchCanRetryWithoutLosingRecords(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`CREATE TRIGGER reject_history BEFORE INSERT ON probe_history BEGIN SELECT RAISE(ABORT, '模拟磁盘写入故障'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(Record{TunnelUUID: "a", Kind: "frame", Status: "normal"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(context.Background()); err == nil {
		t.Fatal("写入失败应返回错误")
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_history"); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	page, err := s.Query(context.Background(), Filter{TunnelUUID: "a"})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("恢复后不能丢失或重复先前结果：%+v %v", page, err)
	}
}
