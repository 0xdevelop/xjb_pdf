# xjb_pdf
Xeno Junction Bridge PDF Tools

Markdown 渲染成 PDF 的 Go 库。中文字体内嵌，渲染文字不联网，调用只有一行。

## 用法

```go
import "github.com/0xdevelop/xjb_pdf"

data, err := xjb_pdf.MarkdownToPDF(ctx, markdown)
```

进 markdown 字节，出 PDF 字节。这个兼容入口默认使用 A4 纵向与内嵌中文字体。渲染核心、字体资源和调用方版式分开；所有字体槽必须使用已注册的本地字体或 PDF 内建字体，避免上游自动下载字体。

支持 CommonMark + GFM（表格、删除线、任务列表、自动链接），代码块带语法高亮。

## 结构

```
xjb_pdf.go     模块门面：版本 + 对外入口，只做转发
xp_markdown/   原 MarkdownToPDF 入口，默认组合内嵌字体插件后转发
xp_render/     唯一 Markdown → PDF 渲染链、Extension 接口、本地字体插件与节点渲染
              不引用 xp_fonts，不嵌入字库；调用方可直接导入
调用方插件     品牌、封面、页眉页脚、版式、专用渲染节点；不进入本库
xp_fonts/      可选 EmbeddedExtension + 内嵌字体；保留原字体注册入口
xp_config/     项目名与版本号（build.sh / git_tag.sh 读这里）
```

## Extend 插件

扩展按调用顺序执行，没有全局注册表、动态插件加载或第二套渲染器。`Extension.Extend(*xp_render.Config) error` 配置一次渲染的页面、样式、节点与 `Prepare` 钩子；各次调用独立，插件不要保存或并发修改本次 Config。Prepare 在创建 writer 后、正文渲染前按序执行，可注册字体或绘制封面。页眉页脚使用 writer 既有接口；特定节点通过 `NodeRenderers` 覆盖，优先级小于 100 可覆盖内置图片/表格节点。插件如自行读取外部资源，其 I/O 由调用方负责。

保持内嵌中文默认值并扩展版式：

```go
layout := xp_render.ExtensionFunc(func(c *xp_render.Config) error {
    c.Page.Orientation = "L"
    c.Styles.Normal.Size = 10
    c.Styles.Normal.Spacing = 7
    return nil
})
data, err := xjb_pdf.MarkdownToPDFWithExtensions(ctx, markdown, layout)
```

轻量入口直接导入 `github.com/0xdevelop/xjb_pdf/xp_render`，默认使用 PDF 内建 Latin 字体；它不引用本库内嵌的中文字体。需要中文时由调用项目提供静态 TTF，也可显式选用 `xp_fonts.EmbeddedExtension{}`：

```go
faces := xp_render.FontFaces{
    Family: "ReportSans", Regular: regularTTF, Bold: boldTTF,
}
data, err := xp_render.MarkdownToPDF(ctx, markdown, faces, layout)
```

品牌文字、标志、报告元数据、数据校验和发布决策由调用项目管理。本库不内置 Transo 或 DD 的业务规则。外部字体资源的许可也由提供方确认。原根包入口仍会链接内嵌字体；直接导入轻量包才可省去这部分资源。模块下载仍会包含仓库字体文件，不等于二进制或 PDF 文件大小。

## 字体

`xp_fonts/` 内嵌 Noto Sans SC 的 Regular 与 Bold 两个字重（SIL Open Font License 1.1，许可证全文见 `xp_fonts/OFL.txt`）。CJK 字形没有斜体设计，斜体槽位复用对应正体。

替换字体必须用**静态 TTF**：底层写入器直接解析 TrueType 的 `glyf` / `loca` 表，读不了 CFF/OpenType（`.otf`）和可变字体。

每份 PDF 只嵌入实际用到的字形（按用到的码位子集化），所以文档体积跟字库大小无关——一份中英混排的样例报告约 40 KB。

## 图片

只画文档自带的内联图（`data:image/...` 形式的 data URI），按图片自身像素尺寸落版；只有超出版心宽或页面可用高度时才等比缩小，小图不会被放大到整栏。

markdown 里指向 `http://` / `https://` 的图片**不会去拉**——底层渲染器默认会在绘制时对这类地址发 HTTP 请求，本库换掉了它的图片渲染分支。PDF 本身没有「阅读时按 URL 取图显示」的机制（规范里的外部图像流各家阅读器都不去取，能取外部资源的 RichMedia / 内嵌 JS 早被封），所以远程图渲染成 **alt 文本 + 链接注解**：文档里没有这张图，点链接由读者的浏览器打开原图，渲染过程一次请求都不发。

其余情况一律只留 alt 文本，不做链接：相对路径、`javascript:` 之类的 scheme（markdown 常是不可信输入，这类地址不该被写进链接注解）、以及底层写入器解不开的内联图（截断的 PNG、交错 PNG、SVG 负载）。一张坏图只丢自己那一块，不会让整篇渲染失败。

表格单元格里的图片只保留 alt 文本，不成链接。表格整体通过同一节点扩展接口渲染：局部状态、中文按字符换行、跨页重复表头，不使用上游表格的包级可变状态。

由此「渲染不联网」是整体成立的，不只是字体那一半。

## 依赖

`github.com/stephenafamo/goldmark-pdf`（版面）、`github.com/yuin/goldmark`（解析），间接带入 chroma（代码高亮）与 gofpdf（PDF 写入）。全部纯 Go，无 CGO，不依赖任何外部命令行工具。不是零依赖库；本次扩展复用这些依赖，没有增加另一套 PDF 引擎。

## License

BSD 3-Clause，见 `LICENSE`。内嵌字体单独适用 OFL 1.1。
