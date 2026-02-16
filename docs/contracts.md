# API / CLI 最小契约

## API
- /version, /healthz
- /chats, /chats/{chat_id}, /chats/batch-delete
- /agent/process
- /cron/jobs 系列
- /models 系列
- /envs 系列
- /skills 系列
- /workspace/files, /workspace/files/{file_path}
- /workspace/export, /workspace/import
- /config/channels 系列

## CLI
- copaw app start
- copaw chats list/create/get/delete/send
- copaw cron list/create/update/delete/pause/resume/run/state
- copaw models list/config/active-get/active-set
- copaw env list/set/delete
- copaw skills list/create/enable/disable/delete
- copaw workspace ls/cat/put/rm/export/import
- copaw channels list/types/get/set

## /agent/process 多步 Agent 协议

`POST /agent/process` 支持两种模式：

1. 常规对话（模型自治多步）
2. 显式工具调用（`biz_params.tool` 单步执行）

请求示例：

```json
{
  "input": [
    {
      "role": "user",
      "type": "message",
      "content": [{ "type": "text", "text": "请读取配置并给出结论" }]
    }
  ],
  "session_id": "s1",
  "user_id": "u1",
  "channel": "console",
  "stream": true
}
```

`stream=false` 返回：

```json
{
  "reply": "最终回复文本",
  "events": [
    { "type": "step_started", "step": 1 },
    { "type": "tool_call", "step": 1, "tool_call": { "name": "shell" } },
    { "type": "tool_result", "step": 1, "tool_result": { "name": "shell", "ok": true, "summary": "..." } },
    { "type": "assistant_delta", "step": 2, "delta": "..." },
    { "type": "completed", "step": 2, "reply": "最终回复文本" }
  ]
}
```

`stream=true` 返回 SSE，`data` payload 与上面 `events` 同构，并以 `data: [DONE]` 结束。

事件类型：

- `step_started`
- `tool_call`
- `tool_result`
- `assistant_delta`
- `completed`
