# 任务 10：存储层

**目标：** 使用 SQLite 实现执行历史和 skill 元数据的持久化存储。

**文件：**
- 创建：`internal/storage/db.go`
- 创建：`internal/storage/history.go`
- 创建：`internal/storage/history_test.go`

---

## 步骤

- [ ] **步骤 1：编写失败测试**

创建 `internal/storage/history_test.go`：

```go
package storage

import (
	"Agent/internal/skill"
	"testing"
	"time"
)

func TestDBInit(t *testing.T) {
	db, err := NewInMemoryDB()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer db.Close()
}

func TestRecordExecution(t *testing.T) {
	db, err := NewInMemoryDB()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer db.Close()

	record := skill.ExecutionRecord{
		SkillPath:    "/test/skill.md",
		SkillVersion: 1,
		Status:       "pass",
		StepsTotal:   3,
		StepsPassed:  3,
		Retries:      0,
		ModelUsed:    "deepseek-v3",
		DurationMs:   1234,
		CreatedAt:    time.Now(),
	}

	id, err := db.RecordExecution(record)
	if err != nil {
		t.Fatalf("记录执行失败: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestGetHistory(t *testing.T) {
	db, err := NewInMemoryDB()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		_, err := db.RecordExecution(skill.ExecutionRecord{
			SkillPath:    "/test/skill.md",
			SkillVersion: 1,
			Status:       "pass",
			StepsTotal:   2,
			StepsPassed:  2,
			CreatedAt:    time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	history, err := db.GetHistory("/test/skill.md", 3)
	if err != nil {
		t.Fatalf("获取历史失败: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 records, got %d", len(history))
	}
}

func TestUpdateSkillMeta(t *testing.T) {
	db, err := NewInMemoryDB()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer db.Close()

	meta := skill.SkillMeta{
		Path:        "/test/skill.md",
		Name:        "test-skill",
		Version:     1,
		TotalTests:  10,
		PassedTests: 8,
		SuccessRate: 0.8,
		LastTestedAt: time.Now(),
	}

	if err := db.UpsertSkillMeta(meta); err != nil {
		t.Fatalf("更新 skill 元数据失败: %v", err)
	}

	loaded, err := db.GetSkillMeta("/test/skill.md")
	if err != nil {
		t.Fatalf("获取 skill 元数据失败: %v", err)
	}

	if loaded.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", loaded.Name)
	}
	if loaded.SuccessRate != 0.8 {
		t.Errorf("expected success rate 0.8, got %f", loaded.SuccessRate)
	}
}

func TestListSkills(t *testing.T) {
	db, err := NewInMemoryDB()
	if err != nil {
		t.Fatalf("创建内存数据库失败: %v", err)
	}
	defer db.Close()

	skills := []skill.SkillMeta{
		{Path: "/a/skill.md", Name: "skill-a", Version: 1, TotalTests: 5, PassedTests: 5, SuccessRate: 1.0, LastTestedAt: time.Now()},
		{Path: "/b/skill.md", Name: "skill-b", Version: 2, TotalTests: 10, PassedTests: 7, SuccessRate: 0.7, LastTestedAt: time.Now()},
	}

	for _, s := range skills {
		if err := db.UpsertSkillMeta(s); err != nil {
			t.Fatal(err)
		}
	}

	list, err := db.ListSkills()
	if err != nil {
		t.Fatalf("列出 skill 失败: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 skills, got %d", len(list))
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./internal/storage/ -v
```
预期：FAIL

- [ ] **步骤 3：实现 DB 层**

创建 `internal/storage/db.go`：

```go
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB 封装 SQLite 数据库。
type DB struct {
	conn *sql.DB
}

// NewDB 打开或创建 SQLite 数据库。
func NewDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return &DB{conn: conn}, nil
}

// NewInMemoryDB 创建内存数据库（用于测试）。
func NewInMemoryDB() (*DB, error) {
	conn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("打开内存数据库失败: %w", err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return &DB{conn: conn}, nil
}

// Close 关闭数据库连接。
func (d *DB) Close() error {
	return d.conn.Close()
}

func migrate(conn *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS execution_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			skill_path TEXT NOT NULL,
			skill_version INTEGER DEFAULT 0,
			status TEXT DEFAULT '',
			steps_total INTEGER DEFAULT 0,
			steps_passed INTEGER DEFAULT 0,
			retries INTEGER DEFAULT 0,
			model_used TEXT DEFAULT '',
			duration_ms INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			error_log TEXT DEFAULT '',
			analysis TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS skills (
			path TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			version INTEGER DEFAULT 0,
			total_tests INTEGER DEFAULT 0,
			passed_tests INTEGER DEFAULT 0,
			success_rate REAL DEFAULT 0.0,
			last_tested_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, q := range queries {
		if _, err := conn.Exec(q); err != nil {
			return fmt.Errorf("迁移失败: %w", err)
		}
	}

	return nil
}
```

- [ ] **步骤 4：实现历史 CRUD**

创建 `internal/storage/history.go`：

```go
package storage

import (
	"Agent/internal/skill"
	"fmt"
)

// RecordExecution 保存执行记录，返回 ID。
func (d *DB) RecordExecution(record skill.ExecutionRecord) (int64, error) {
	result, err := d.conn.Exec(
		`INSERT INTO execution_history
		(skill_path, skill_version, status, steps_total, steps_passed, retries, model_used, duration_ms, created_at, error_log, analysis)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.SkillPath, record.SkillVersion, record.Status,
		record.StepsTotal, record.StepsPassed, record.Retries,
		record.ModelUsed, record.DurationMs, record.CreatedAt,
		record.ErrorLog, record.Analysis,
	)
	if err != nil {
		return 0, fmt.Errorf("记录执行失败: %w", err)
	}
	return result.LastInsertId()
}

// GetHistory 返回指定 skill 的最近 N 条执行记录。
func (d *DB) GetHistory(skillPath string, limit int) ([]skill.ExecutionRecord, error) {
	rows, err := d.conn.Query(
		`SELECT id, skill_path, skill_version, status, steps_total, steps_passed, retries, model_used, duration_ms, created_at, error_log, analysis
		FROM execution_history
		WHERE skill_path = ?
		ORDER BY id DESC
		LIMIT ?`,
		skillPath, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("查询历史失败: %w", err)
	}
	defer rows.Close()

	var records []skill.ExecutionRecord
	for rows.Next() {
		var r skill.ExecutionRecord
		if err := rows.Scan(
			&r.ID, &r.SkillPath, &r.SkillVersion, &r.Status,
			&r.StepsTotal, &r.StepsPassed, &r.Retries,
			&r.ModelUsed, &r.DurationMs, &r.CreatedAt,
			&r.ErrorLog, &r.Analysis,
		); err != nil {
			return nil, fmt.Errorf("扫描记录失败: %w", err)
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// UpsertSkillMeta 插入或更新 skill 元数据。
func (d *DB) UpsertSkillMeta(meta skill.SkillMeta) error {
	_, err := d.conn.Exec(
		`INSERT INTO skills (path, name, version, total_tests, passed_tests, success_rate, last_tested_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			name = excluded.name,
			version = excluded.version,
			total_tests = excluded.total_tests,
			passed_tests = excluded.passed_tests,
			success_rate = excluded.success_rate,
			last_tested_at = excluded.last_tested_at`,
		meta.Path, meta.Name, meta.Version,
		meta.TotalTests, meta.PassedTests, meta.SuccessRate,
		meta.LastTestedAt,
	)
	if err != nil {
		return fmt.Errorf("更新 skill 元数据失败: %w", err)
	}
	return nil
}

// GetSkillMeta 按路径返回 skill 元数据。
func (d *DB) GetSkillMeta(skillPath string) (*skill.SkillMeta, error) {
	var meta skill.SkillMeta
	err := d.conn.QueryRow(
		`SELECT path, name, version, total_tests, passed_tests, success_rate, last_tested_at
		FROM skills WHERE path = ?`, skillPath,
	).Scan(
		&meta.Path, &meta.Name, &meta.Version,
		&meta.TotalTests, &meta.PassedTests, &meta.SuccessRate,
		&meta.LastTestedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("skill 元数据未找到: %w", err)
	}
	return &meta, nil
}

// ListSkills 返回所有已追踪的 skill。
func (d *DB) ListSkills() ([]skill.SkillMeta, error) {
	rows, err := d.conn.Query(
		`SELECT path, name, version, total_tests, passed_tests, success_rate, last_tested_at
		FROM skills ORDER BY last_tested_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("列出 skill 失败: %w", err)
	}
	defer rows.Close()

	var skills []skill.SkillMeta
	for rows.Next() {
		var s skill.SkillMeta
		if err := rows.Scan(
			&s.Path, &s.Name, &s.Version,
			&s.TotalTests, &s.PassedTests, &s.SuccessRate,
			&s.LastTestedAt,
		); err != nil {
			return nil, fmt.Errorf("扫描 skill 失败: %w", err)
		}
		skills = append(skills, s)
	}

	return skills, rows.Err()
}
```

- [ ] **步骤 5：运行测试确认通过**

```bash
go test ./internal/storage/ -v
```
预期：PASS

- [ ] **步骤 6：提交**

```bash
git add internal/storage/
git commit -m "feat: 添加 SQLite 存储层，支持执行历史和 skill 元数据"
```
