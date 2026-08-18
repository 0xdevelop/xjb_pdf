# xjb_pdf

Markdown 渲染成 PDF 的 Go 库。中文字体内嵌，渲染文字不联网，调用只有一行。

## 用法

```go
import "github.com/0xdevelop/xjb_pdf"

data, err := xjb_pdf.RenderMarkdown(ctx, markdown)
```

进 markdown 字节，出 PDF 字节。纸张 A4 纵向、页边距、字号层级、字体全部在包内固定——不开配置口子是刻意的：底层渲染器对任何没被显式指定字体的样式槽会回落到 Google 托管字体并在渲染时下载，把所有槽位锁死在内嵌字体上，才谈得上离线。

支持 CommonMark + GFM（表格、删除线、任务列表、自动链接），代码块带语法高亮。

## 结构

```
xjb_pdf.go     模块门面：版本 + 对外入口，只做转发
xp_markdown/   markdown → PDF 的渲染实现（xp_markdown.Render）
xp_fonts/      内嵌字体与样式槽注册
xp_config/     项目名与版本号（build.sh / git_tag.sh 读这里）
```

## 字体

`xp_fonts/` 内嵌 Noto Sans SC 的 Regular 与 Bold 两个字重（SIL Open Font License 1.1，许可证全文见 `xp_fonts/OFL.txt`）。CJK 字形没有斜体设计，斜体槽位复用对应正体。

替换字体必须用**静态 TTF**：底层写入器直接解析 TrueType 的 `glyf` / `loca` 表，读不了 CFF/OpenType（`.otf`）和可变字体。

每份 PDF 只嵌入实际用到的字形（按用到的码位子集化），所以文档体积跟字库大小无关——一份中英混排的样例报告约 40 KB。

## 图片

markdown 里目标为 `http://` / `https://` 的图片，会由底层渲染器在绘制时发起 HTTP 请求拉取。渲染不联网这条只覆盖字体，不覆盖远程图片。取不到图不会让渲染失败，图片位置留空。

## 依赖

`github.com/stephenafamo/goldmark-pdf`（版面）、`github.com/yuin/goldmark`（解析），间接带入 chroma（代码高亮）与 gofpdf（PDF 写入）。全部纯 Go，无 CGO，不依赖任何外部命令行工具。

## License

BSD 3-Clause，见 `LICENSE`。内嵌字体单独适用 OFL 1.1。
