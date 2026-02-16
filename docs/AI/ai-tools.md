# AI Tool Guide (Gateway)

你可以通过 `POST /agent/process` 触发工具调用。

## 工具

- `shell`：执行 shell 命令（高风险）。

## 调用格式

通过 `biz_params.tool` 直接调用工具（当前仅支持 `shell`）：

```json
{
  "biz_params": {
    "tool": {
      "name": "shell",
      "input": { "command": "pwd" }
    }
  }
}
```

## 安全边界

- 非必要不要使用 `shell`。
