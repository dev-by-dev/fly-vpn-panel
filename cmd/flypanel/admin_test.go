package main

import (
	"testing"

	"flypanel/internal/auth"
	"flypanel/internal/store"
)

func TestAdminStatus(t *testing.T) {
	dir := t.TempDir()
	exists, err := adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(empty): %v", err)
	}
	if exists {
		t.Fatal("adminStatus(empty) = true, want false")
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	if err := auth.AdminInit(st, "admin", "FlyPanel-2026x"); err != nil {
		t.Fatalf("AdminInit(): %v", err)
	}
	st.Close()

	exists, err = adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(seeded): %v", err)
	}
	if !exists {
		t.Fatal("adminStatus(seeded) = false, want true")
	}
}
