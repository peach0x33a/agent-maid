package meta

import (
	"os"
	"path/filepath"
	"testing"
)

// setupIsolated 把旧工具目录与 agent-maid 目录都指到临时目录。
func setupIsolated(t *testing.T) (legacy, maid string) {
	t.Helper()
	base := t.TempDir()
	legacy = filepath.Join(base, "legacy-config")
	maid = filepath.Join(base, "maid")
	t.Setenv("XDG_CONFIG_HOME", legacy)
	t.Setenv("AGENT_MAID_DIR", maid)
	t.Setenv("ZCODE_ACC_SWITCH_DIR", "") // 清掉本机可能存在的覆盖
	return legacy, maid
}

func writeLegacySlot(t *testing.T, legacy, name string) {
	t.Helper()
	dir := filepath.Join(legacy, "zcode-acc-switch", "slots")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(`{"name":"`+name+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 用户报告的场景:迁移来的槽位在新位置删除后,重启程序又被旧目录复制回来。
func TestMigrateLegacyDoesNotResurrectDeletedSlots(t *testing.T) {
	legacy, maid := setupIsolated(t)
	writeLegacySlot(t, legacy, "6090")

	MigrateLegacy()

	migrated := filepath.Join(maid, "zcode", "slots", "6090.json")
	if _, err := os.Stat(migrated); err != nil {
		t.Fatalf("首次迁移应复制槽位: %v", err)
	}

	if err := os.Remove(migrated); err != nil {
		t.Fatal(err)
	}
	MigrateLegacy() // 模拟重启

	if _, err := os.Stat(migrated); !os.IsNotExist(err) {
		t.Fatal("已删除的槽位在重启后被旧目录复活")
	}
}

// 新目录已经在用(修复前就用过本程序)时,直接视为迁移完成。
func TestMigrateLegacySkipsWhenTargetInUse(t *testing.T) {
	legacy, maid := setupIsolated(t)
	writeLegacySlot(t, legacy, "6090")

	// 用户已经在 agent-maid 里管理数据:新槽位目录存在
	inUse := filepath.Join(maid, "zcode", "slots")
	if err := os.MkdirAll(inUse, 0o700); err != nil {
		t.Fatal(err)
	}

	MigrateLegacy()

	if _, err := os.Stat(filepath.Join(inUse, "6090.json")); !os.IsNotExist(err) {
		t.Fatal("新目录在用时应跳过迁移,而不是把旧槽位复制回来")
	}
	if _, err := os.Stat(filepath.Join(maid, migrateMarker)); err != nil {
		t.Fatal("跳过迁移时应补写完成标记")
	}
}

// 正常路径:首次运行迁移,之后标记存在则不再执行任何复制。
func TestMigrateLegacyRunsOnce(t *testing.T) {
	legacy, maid := setupIsolated(t)
	writeLegacySlot(t, legacy, "a")
	writeLegacySlot(t, legacy, "b")

	MigrateLegacy()
	MigrateLegacy() // 第二次不应有副作用

	slots := filepath.Join(maid, "zcode", "slots")
	for _, name := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(slots, name+".json")); err != nil {
			t.Fatalf("槽位 %s 应已迁移: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(maid, migrateMarker)); err != nil {
		t.Fatal("迁移后应留下完成标记")
	}
}
