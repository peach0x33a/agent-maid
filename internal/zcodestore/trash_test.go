package zcodestore

import (
	"os"
	"path/filepath"
	"testing"

	"agent-maid/internal/zcode"
)

func TestTrashRoundtrip(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if _, err := s.Save("work", Slot{Name: "work", Credentials: zcode.Credentials{"k": "v"}}); err != nil {
		t.Fatal(err)
	}

	trashName, err := s.DeleteToTrash("work")
	if err != nil {
		t.Fatalf("DeleteToTrash: %v", err)
	}
	if _, err := s.Get("work"); !os.IsNotExist(err) {
		t.Fatal("删除后槽位应不存在")
	}
	if _, err := os.Stat(filepath.Join(s.TrashDir(), trashName)); err != nil {
		t.Fatalf("回收站文件应存在: %v", err)
	}

	name, err := s.RestoreLatestFromTrash()
	if err != nil {
		t.Fatalf("RestoreLatestFromTrash: %v", err)
	}
	if name != "work" {
		t.Fatalf("恢复的槽位名 = %q, 期望 work", name)
	}
	if _, err := s.Get("work"); err != nil {
		t.Fatalf("恢复后槽位应存在: %v", err)
	}
}

func TestPurgeTrash(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if _, err := s.Save("work", Slot{Name: "work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteToTrash("work"); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeTrash(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreLatestFromTrash(); err == nil {
		t.Fatal("清空后不应可恢复")
	}
}

func TestRestoreSkipsCorruptFiles(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if err := os.MkdirAll(s.TrashDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.TrashDir(), "1-bad.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreLatestFromTrash(); err == nil {
		t.Fatal("回收站只有损坏文件时应报错")
	}
}
