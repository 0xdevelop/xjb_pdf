# xjb_pdf
Xeno Junction Bridge PDF Tools

Markdown 渲染成 PDF 的 Go 库。中文字体内嵌，渲染文字不联网，调用只有一行。

## 用法

```go
import "github.com/0xdevelop/xjb_pdf"

data, err := xjb_pdf.MarkdownToPDF(ctx, markdown)
```

进 markdown 字节，出 PDF 字节。纸张 A4 纵向、页边距、字号层级、字体全部在包内固定——不开配置口子是刻意的：底层渲染器对任何没被显式指定字体的样式槽会回落到 Google 托管字体并在渲染时下载，把所有槽位锁死在内嵌字体上，才谈得上离线。

支持 CommonMark + GFM（表格、删除线、任务列表、自动链接），代码块带语法高亮。

## 结构

```
xjb_pdf.go     模块门面：版本 + 对外入口，只做转发
xp_markdown/   markdown → PDF 的渲染实现（xp_markdown.MarkdownToPDF）与不外发的图片渲染分支
xp_fonts/      内嵌字体（xp_fonts.RegisterEmbeddedFaces / EmbeddedFace）与样式槽注册
xp_config/     项目名与版本号（build.sh / git_tag.sh 读这里）
```

## 字体

`xp_fonts/` 内嵌 Noto Sans SC 的 Regular 与 Bold 两个字重（SIL Open Font License 1.1，许可证全文见 `xp_fonts/OFL.txt`）。CJK 字形没有斜体设计，斜体槽位复用对应正体。

替换字体必须用**静态 TTF**：底层写入器直接解析 TrueType 的 `glyf` / `loca` 表，读不了 CFF/OpenType（`.otf`）和可变字体。

每份 PDF 只嵌入实际用到的字形（按用到的码位子集化），所以文档体积跟字库大小无关——一份中英混排的样例报告约 40 KB。

## 图片

只画文档自带的内联图（`data:image/...` 形式的 data URI），按图片自身像素尺寸落版；只有超出版心宽或页面可用高度时才等比缩小，小图不会被放大到整栏。

markdown 里指向 `http://` / `https://` 的图片**不会去拉**——底层渲染器默认会在绘制时对这类地址发 HTTP 请求，本库换掉了它的图片渲染分支。PDF 本身没有「阅读时按 URL 取图显示」的机制（规范里的外部图像流各家阅读器都不去取，能取外部资源的 RichMedia / 内嵌 JS 早被封），所以远程图渲染成 **alt 文本 + 链接注解**：文档里没有这张图，点链接由读者的浏览器打开原图，渲染过程一次请求都不发。

其余情况一律只留 alt 文本，不做链接：相对路径、`javascript:` 之类的 scheme（markdown 常是不可信输入，这类地址不该被写进链接注解）、以及底层写入器解不开的内联图（截断的 PNG、交错 PNG、SVG 负载）。一张坏图只丢自己那一块，不会让整篇渲染失败。

表格单元格里的图片是例外：单元格内容由上游的表格渲染分支自己抽取，图片节点走不到本库的渲染函数，只会留下 alt 文本，不成链接。

由此「渲染不联网」是整体成立的，不只是字体那一半。

## 依赖

`github.com/stephenafamo/goldmark-pdf`（版面）、`github.com/yuin/goldmark`（解析），间接带入 chroma（代码高亮）与 gofpdf（PDF 写入）。全部纯 Go，无 CGO，不依赖任何外部命令行工具。

## License

BSD 3-Clause，见 `LICENSE`。内嵌字体单独适用 OFL 1.1。
