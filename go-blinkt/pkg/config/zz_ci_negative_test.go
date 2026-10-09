package config

import "testing"

// Throwaway: proves CI fails on a broken test (adopt-go-ci task 3.2). Reverted.
func TestCINegative(t *testing.T) { t.Fatal("deliberate CI failure") }
