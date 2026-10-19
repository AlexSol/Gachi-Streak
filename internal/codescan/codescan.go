// Package codescan finds Go constructs in the lines a commit added.
// Files are parsed whole (go/ast), but a construct only counts when the
// line it starts on was added, so old code never unlocks anything.
// No I/O here, so it is fully unit-testable.
package codescan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// Kinds of findings. They match the achievement IDs they feed.
const (
	Goroutine = "goroutine"  // go statement
	Channel   = "channel"    // make(chan T)
	Select    = "select"     // select with >= 2 cases
	Context   = "context"    // context.WithCancel / WithTimeout / WithDeadline
	Generics  = "generics"   // func with type parameters
	ErrWrap   = "errwrap"    // fmt.Errorf("…%w…"), counted per call
	FirstTest = "first_test" // func TestXxx(t *testing.T) in *_test.go
	Benchmark = "benchmark"  // func BenchmarkXxx(b *testing.B)
	TableTest = "table_test" // []struct{…} cases + t.Run in one test func
	Fuzz      = "fuzz"       // func FuzzXxx(f *testing.F)
	NilMap    = "nil_map"    // var m map[K]V never made, then m[k] = v
)

// File is one changed Go file: its full new source and the added lines.
type File struct {
	Path  string
	Src   []byte
	Added map[int]bool // 1-based line numbers in the new file
}

// Analyze returns how many times each kind occurs in added lines.
// Files that do not parse are skipped.
func Analyze(files []File) map[string]int {
	out := map[string]int{}
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") || len(f.Added) == 0 {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f.Path, f.Src, 0)
		if err != nil {
			continue
		}
		s := &scan{fset: fset, added: f.Added, out: out}
		s.file(file, strings.HasSuffix(f.Path, "_test.go"))
	}
	return out
}

type scan struct {
	fset  *token.FileSet
	added map[int]bool
	out   map[string]int
}

func (s *scan) isAdded(n ast.Node) bool { return s.added[s.fset.Position(n.Pos()).Line] }

func (s *scan) hit(kind string, n ast.Node) {
	if s.isAdded(n) {
		s.out[kind]++
	}
}

func (s *scan) file(f *ast.File, isTest bool) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GoStmt:
			s.hit(Goroutine, n)
		case *ast.SelectStmt:
			if len(n.Body.List) >= 2 {
				s.hit(Select, n)
			}
		case *ast.CallExpr:
			s.call(n)
		case *ast.FuncDecl:
			if n.Type.TypeParams != nil && n.Type.TypeParams.NumFields() > 0 {
				s.hit(Generics, n)
			}
			if isTest {
				s.testFunc(n)
			}
			if n.Body != nil {
				s.nilMaps(n.Body)
			}
		case *ast.FuncLit:
			s.nilMaps(n.Body)
		}
		return true
	})
}

func (s *scan) call(c *ast.CallExpr) {
	if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "make" && len(c.Args) > 0 {
		if _, ok := c.Args[0].(*ast.ChanType); ok {
			s.hit(Channel, c)
		}
		return
	}
	pkg, name, ok := selector(c.Fun)
	if !ok {
		return
	}
	switch {
	case pkg == "context" && (name == "WithCancel" || name == "WithTimeout" || name == "WithDeadline" ||
		name == "WithCancelCause" || name == "WithTimeoutCause" || name == "WithDeadlineCause"):
		s.hit(Context, c)
	case pkg == "fmt" && name == "Errorf" && len(c.Args) > 0:
		if lit, ok := c.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if v, err := strconv.Unquote(lit.Value); err == nil && strings.Contains(v, "%w") {
				s.hit(ErrWrap, c)
			}
		}
	}
}

// testFunc detects Test/Benchmark/Fuzz functions and table-driven tests.
func (s *scan) testFunc(fn *ast.FuncDecl) {
	param := ""
	if ps := fn.Type.Params.List; len(ps) == 1 && len(ps[0].Names) <= 1 {
		if star, ok := ps[0].Type.(*ast.StarExpr); ok {
			if pkg, name, ok := selector(star.X); ok && pkg == "testing" {
				param = name
			}
		}
	}
	name := fn.Name.Name
	switch {
	case param == "T" && strings.HasPrefix(name, "Test"):
		s.hit(FirstTest, fn)
	case param == "B" && strings.HasPrefix(name, "Benchmark"):
		s.hit(Benchmark, fn)
	case param == "F" && strings.HasPrefix(name, "Fuzz"):
		s.hit(Fuzz, fn)
	}
	if param != "T" || fn.Body == nil {
		return
	}
	// Table-driven: a []struct{…} literal and a t.Run(name, func…) call in
	// the same test; counts once when either part was added.
	var table, run ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if at, ok := n.Type.(*ast.ArrayType); ok && at.Len == nil {
				if _, ok := at.Elt.(*ast.StructType); ok && table == nil {
					table = n
				}
			}
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Run" && len(n.Args) == 2 {
				if _, ok := n.Args[1].(*ast.FuncLit); ok && run == nil {
					run = n
				}
			}
		}
		return true
	})
	if table != nil && run != nil && (s.isAdded(table) || s.isAdded(run)) {
		s.out[TableTest]++
	}
}

// nilMaps finds `var m map[K]V` without a value that is written by index
// (m[k] = v) and never assigned as a whole: a guaranteed runtime panic.
// Nested function literals are left to their own pass.
func (s *scan) nilMaps(body *ast.BlockStmt) {
	nilVars := map[*ast.Object]bool{}
	assigned := map[*ast.Object]bool{}
	var writes []struct {
		obj  *ast.Object
		node ast.Node
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ValueSpec:
			if _, ok := n.Type.(*ast.MapType); ok && len(n.Values) == 0 {
				for _, id := range n.Names {
					if id.Obj != nil {
						nilVars[id.Obj] = true
					}
				}
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				switch l := lhs.(type) {
				case *ast.Ident:
					if l.Obj != nil {
						assigned[l.Obj] = true
					}
				case *ast.IndexExpr:
					if id, ok := l.X.(*ast.Ident); ok && id.Obj != nil {
						writes = append(writes, struct {
							obj  *ast.Object
							node ast.Node
						}{id.Obj, n})
					}
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND { // &m may be filled elsewhere
				if id, ok := n.X.(*ast.Ident); ok && id.Obj != nil {
					assigned[id.Obj] = true
				}
			}
		}
		return true
	})
	for _, w := range writes {
		if nilVars[w.obj] && !assigned[w.obj] && s.isAdded(w.node) {
			s.out[NilMap]++
			return
		}
	}
}

// selector splits pkg.Name.
func selector(e ast.Expr) (pkg, name string, ok bool) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return id.Name, sel.Sel.Name, true
}

// AddedLines parses a unified diff hunk list (as GitHub returns in
// files[].patch) into the 1-based line numbers added to the new file.
func AddedLines(patch string) map[int]bool {
	out := map[int]bool{}
	line := 0
	for _, l := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			// @@ -a,b +c,d @@
			if i := strings.Index(l, "+"); i >= 0 {
				rest := l[i+1:]
				end := strings.IndexAny(rest, ", ")
				if end < 0 {
					end = len(rest)
				}
				n, err := strconv.Atoi(rest[:end])
				if err != nil {
					line = 0
					continue
				}
				line = n
			}
		case line == 0:
			// before the first hunk
		case strings.HasPrefix(l, "+"):
			out[line] = true
			line++
		case strings.HasPrefix(l, "-"), strings.HasPrefix(l, `\`):
			// removed line or "\ No newline at end of file"
		default:
			line++
		}
	}
	return out
}
