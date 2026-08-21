package xp_markdown

import (
	"bytes"
	"context"
	"testing"
)

const renderSampleMarkdown = `# 湿实验交付报告 / Wet-lab Report

## 概要 Summary

本轮筛选共产出 **12** 条候选序列，其中 3 条进入 *复筛*。Mixed 中英文 line wrapping check.

| 编号 | 序列 | 结论 |
|------|------|------|
| 001  | ACGT | Go   |
| 002  | TGCA | 待定 |

### 复现步骤

1. 取样并编号
2. 按协议孵育 4 小时
3. 读板

- 冷链全程 4°C
- 复孔数 n=3

` + "```" + `go
func main() {
	fmt.Println("样本导出完成")
}
` + "```" + `
`

func TestMarkdownToPDF(t *testing.T) {
	out, err := MarkdownToPDF(context.Background(), []byte(renderSampleMarkdown))
	if err != nil {
		t.Fatalf("MarkdownToPDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output is not a PDF document, first bytes: %q", out[:min(8, len(out))])
	}
	// A composite font object proves the embedded TrueType face was used
	// instead of a latin-only inbuilt font, which is what CJK text needs.
	if !bytes.Contains(out, []byte("/Type0")) {
		t.Fatal("no composite (/Type0) font in output: CJK glyphs would be blank")
	}
}

func TestMarkdownToPDFRejectsEmptyInput(t *testing.T) {
	if _, err := MarkdownToPDF(context.Background(), []byte("   \n\t")); err == nil {
		t.Fatal("expected an error for blank markdown input")
	}
}

func TestMarkdownToPDFHonoursCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MarkdownToPDF(ctx, []byte("# heading")); err == nil {
		t.Fatal("expected an error for a cancelled context")
	}
}
