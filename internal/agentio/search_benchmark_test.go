package agentio_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sausheong/hand/internal/agentio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func BenchmarkSearchLargeTextFiles(b *testing.B) {
	dir := b.TempDir()
	data := []byte(strings.Repeat("ordinary source line\n", 10000))
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("source%d.txt", i)), data, 0600); err != nil {
			b.Fatal(err)
		}
	}
	search := &agentio.SearchTool{WorkDir: dir}
	b.SetBytes(int64(len(data) * 20))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := search.Execute(context.Background(), json.RawMessage(`{"content":"absent-marker"}`))
		if err != nil || r.Error != "" || r.Metadata["complete"] != true {
			b.Fatalf("incomplete scan: %+v %v", r, err)
		}
	}
}
