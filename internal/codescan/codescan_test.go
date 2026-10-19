package codescan

import (
	"reflect"
	"strings"
	"testing"
)

// all marks every line of src as added.
func all(path, src string) File {
	added := map[int]bool{}
	for i := range strings.Count(src, "\n") + 1 {
		added[i+1] = true
	}
	return File{Path: path, Src: []byte(src), Added: added}
}

func TestAnalyzeConstructs(t *testing.T) {
	src := `package x

import (
	"context"
	"fmt"
	"time"
)

func Map[T any](in []T, f func(T) T) []T { return in }

func run(err error) error {
	ch := make(chan int)
	done := make(chan struct{})
	go func() { ch <- 1 }()
	select {
	case <-ch:
	case <-done:
	}
	select {
	case <-ch:
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = ctx
	_ = fmt.Errorf("a: %w", err)
	_ = fmt.Errorf("b: %v", err)
	return fmt.Errorf("c: %w", err)
}

func bad() {
	var m map[string]int
	m["x"] = 1
	var ok map[string]int
	ok = map[string]int{}
	ok["y"] = 2
}
`
	got := Analyze([]File{all("x.go", src)})
	want := map[string]int{Generics: 1, Channel: 2, Goroutine: 1, Select: 1, Context: 1, ErrWrap: 2, NilMap: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAnalyzeTests(t *testing.T) {
	src := `package x

import "testing"

func TestAdd(t *testing.T) {
	cases := []struct{ a, b, want int }{{1, 2, 3}}
	for _, c := range cases {
		t.Run("x", func(t *testing.T) { _ = c })
	}
}

func BenchmarkAdd(b *testing.B) {}
func FuzzAdd(f *testing.F)       {}
func TestHelper(x int)           {}
`
	got := Analyze([]File{all("x_test.go", src)})
	want := map[string]int{FirstTest: 1, TableTest: 1, Benchmark: 1, Fuzz: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Test functions outside *_test.go do not count.
	if got := Analyze([]File{all("x.go", src)}); len(got) != 0 {
		t.Fatalf("non-test file: %v", got)
	}
}

func TestOnlyAddedLinesCount(t *testing.T) {
	src := "package x\n\nfunc a() { go a() }\n\nfunc b() { go b() }\n"
	f := File{Path: "x.go", Src: []byte(src), Added: map[int]bool{5: true}}
	if got := Analyze([]File{f}); got[Goroutine] != 1 {
		t.Fatalf("got %v", got)
	}
	f.Added = map[int]bool{1: true}
	if got := Analyze([]File{f}); len(got) != 0 {
		t.Fatalf("old code counted: %v", got)
	}
}

func TestBrokenAndNonGoFilesSkipped(t *testing.T) {
	files := []File{all("x.go", "package x\nfunc {"), all("README.md", "go func() {}()")}
	if got := Analyze(files); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestAddedLines(t *testing.T) {
	patch := `@@ -1,3 +1,4 @@
 package x
-func a() {}
+func a() int { return 1 }
+func b() {}

@@ -10,2 +11,3 @@ func c() {
 	x := 1
+	y := 2
\ No newline at end of file`
	got := AddedLines(patch)
	want := map[int]bool{2: true, 3: true, 12: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A new file: "@@ -0,0 +1,2 @@"
	if got := AddedLines("@@ -0,0 +1,2 @@\n+package x\n+func a() {}"); !got[1] || !got[2] {
		t.Fatalf("new file %v", got)
	}
}
