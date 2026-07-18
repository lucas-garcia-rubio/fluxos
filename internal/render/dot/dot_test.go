package dot

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/render"
)

var errDOTWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errDOTWrite
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func renderString(t *testing.T, snapshot render.Snapshot) string {
	t.Helper()
	var out bytes.Buffer
	if err := Render(&out, snapshot); err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out.String()
}

func TestRenderEmptyGraph(t *testing.T) {
	if got, want := renderString(t, render.Snapshot{}), "digraph fluxos {\n}\n"; got != want {
		t.Fatalf("Render(empty) = %q, want %q", got, want)
	}
}

func TestRenderNodesKindsEdgesAndCycles(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_a", Label: "A.start()", Kind: render.NodeMethod},
			{ID: "m_b", Label: "B.run() [unresolved]", Kind: render.NodeUnresolved},
		},
		Edges: []render.EdgeView{
			{From: "m_a", To: "m_b"},
			{From: "m_a", To: "m_b"},
			{From: "m_b", To: "m_a", Cycle: true},
		},
	}
	got := renderString(t, snapshot)
	if !strings.Contains(got, `"m_a" [label="A.start()", kind="method"];`) {
		t.Fatalf("method node missing:\n%s", got)
	}
	if !strings.Contains(got, `"m_b" [label="B.run() [unresolved]", kind="unresolved"];`) {
		t.Fatalf("terminal node missing:\n%s", got)
	}
	if count := strings.Count(got, `"m_a" -> "m_b";`); count != 2 {
		t.Fatalf("parallel edge count = %d, want 2:\n%s", count, got)
	}
	if !strings.Contains(got, `"m_b" -> "m_a" [color="red", style="dashed"];`) {
		t.Fatalf("cycle edge missing:\n%s", got)
	}
}

func TestRenderPreservesSnapshotOrder(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_b", Label: "B", Kind: render.NodeMethod},
			{ID: "m_a", Label: "A", Kind: render.NodeMethod},
		},
		Edges: []render.EdgeView{
			{From: "m_b", To: "m_a"},
			{From: "m_a", To: "m_b"},
		},
	}
	got := renderString(t, snapshot)
	if strings.Index(got, `"m_b" [`) > strings.Index(got, `"m_a" [`) {
		t.Fatalf("renderer reordered nodes:\n%s", got)
	}
	if strings.Index(got, `"m_b" -> "m_a"`) > strings.Index(got, `"m_a" -> "m_b"`) {
		t.Fatalf("renderer reordered edges:\n%s", got)
	}
}

func TestQuoteEscapesDOTStrings(t *testing.T) {
	input := "quote\" slash\\ line\ncarriage\rtab\t\\N"
	want := "\"quote\\\" slash\\\\ line\\ncarriage\\rtab\t\\\\N\""
	if got := quote(input); got != want {
		t.Fatalf("quote = %q, want %q", got, want)
	}
}

func TestRenderPropagatesWriterErrors(t *testing.T) {
	if err := Render(failingWriter{}, render.Snapshot{}); !errors.Is(err, errDOTWrite) {
		t.Fatalf("Render writer error = %v", err)
	}
	if err := Render(shortWriter{}, render.Snapshot{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Render short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestRenderGolden(t *testing.T) {
	snapshot := render.Snapshot{
		Nodes: []render.NodeView{
			{ID: "m_a", Label: "A.start()", Kind: render.NodeMethod},
			{ID: "m_b", Label: "B.run() [ambiguous: 2 implementations]", Kind: render.NodeAmbiguousImplementation},
			{ID: "m_escape", Label: "Escaped \"quote\" \\path\nnext\rreturn\t\\N", Kind: render.NodeExternal},
		},
		Edges: []render.EdgeView{
			{From: "m_a", To: "m_b"},
			{From: "m_a", To: "m_b"},
			{From: "m_b", To: "m_a", Cycle: true},
		},
	}
	want, err := os.ReadFile("testdata/callgraph.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got := renderString(t, snapshot); got != string(want) {
		t.Fatalf("golden mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderOutputIsAcceptedByGraphviz(t *testing.T) {
	dotPath, err := exec.LookPath("dot")
	if err != nil {
		t.Skip("Graphviz dot is not installed")
	}
	source, err := os.ReadFile("testdata/callgraph.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	cmd := exec.Command(dotPath, "-Tsvg")
	cmd.Stdin = bytes.NewReader(source)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("dot -Tsvg: %v", err)
	}
	if len(output) == 0 {
		t.Fatal("dot -Tsvg produced empty output")
	}
}
