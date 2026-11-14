# 图片预览

图片使用相对路径，以本 Markdown 文件所在目录为基准。

![彩条与棋盘格测试图](assets/local-image.png)

## 引用式图片

![同一张本地图][sample]

[sample]: assets/local-image.png

## 网络图片

![Go 标志](https://go.dev/blog/go-brand/Go-Logo/PNG/Go-Logo_Blue.png)

网络图片在后台下载，完成后自动刷新，切换布局或调整窗口会复用会话缓存。

## 无法加载的图片

![不存在的文件](assets/missing.png)

## 行内图片与代码

行内图片保留文字形式：![测试图](assets/local-image.png)。

```markdown
![图片说明](assets/local-image.png)
```

## 滚动检查

图片应随预览内容一起滚动。切换编辑布局、帮助页或缩小窗口后，图片不会覆盖其他面板。

支持本地和 HTTP/HTTPS 图片，格式包括 PNG、JPEG、GIF 首帧。
