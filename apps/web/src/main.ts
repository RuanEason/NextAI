type Tone = "neutral" | "info" | "error";

interface ChatSpec {
  id: string;
  name: string;
  session_id: string;
  user_id: string;
  channel: string;
  updated_at: string;
}

interface RuntimeContent {
  type?: string;
  text?: string;
}

interface RuntimeMessage {
  id?: string;
  role?: string;
  content?: RuntimeContent[];
}

interface ChatHistoryResponse {
  messages: RuntimeMessage[];
}

interface ErrorEnvelope {
  error?: {
    code?: string;
    message?: string;
    details?: unknown;
  };
}

interface ViewMessage {
  id: string;
  role: "user" | "assistant";
  text: string;
}

const DEFAULT_API_BASE = "http://127.0.0.1:8088";
const DEFAULT_USER_ID = "demo-user";
const DEFAULT_CHANNEL = "console";
const SETTINGS_KEY = "copaw-next.web.chat.settings";

const apiBaseInput = mustElement<HTMLInputElement>("api-base");
const userIdInput = mustElement<HTMLInputElement>("user-id");
const channelInput = mustElement<HTMLInputElement>("channel");
const reloadChatsButton = mustElement<HTMLButtonElement>("reload-chats");
const newChatButton = mustElement<HTMLButtonElement>("new-chat");
const chatList = mustElement<HTMLUListElement>("chat-list");
const chatTitle = mustElement<HTMLElement>("chat-title");
const chatSession = mustElement<HTMLElement>("chat-session");
const messageList = mustElement<HTMLUListElement>("message-list");
const composerForm = mustElement<HTMLFormElement>("composer");
const messageInput = mustElement<HTMLTextAreaElement>("message-input");
const sendButton = mustElement<HTMLButtonElement>("send-btn");
const statusLine = mustElement<HTMLElement>("status-line");

const state = {
  apiBase: DEFAULT_API_BASE,
  userId: DEFAULT_USER_ID,
  channel: DEFAULT_CHANNEL,
  chats: [] as ChatSpec[],
  activeChatId: null as string | null,
  activeSessionId: newSessionID(),
  messages: [] as ViewMessage[],
  sending: false,
};

void bootstrap();

async function bootstrap(): Promise<void> {
  restoreSettings();
  bindEvents();
  renderChatHeader();
  renderMessages();
  setStatus("Loading sessions...", "info");
  await reloadChats();
  if (state.chats.length > 0) {
    await openChat(state.chats[0].id);
    setStatus("Loaded latest session", "info");
    return;
  }
  startDraftSession();
  setStatus("No session yet, draft started", "info");
}

function bindEvents(): void {
  reloadChatsButton.addEventListener("click", async () => {
    syncControlState();
    setStatus("Reloading sessions...", "info");
    await reloadChats();
    setStatus("Sessions refreshed", "info");
  });

  newChatButton.addEventListener("click", () => {
    syncControlState();
    startDraftSession();
    setStatus("Draft session ready", "info");
  });

  apiBaseInput.addEventListener("change", async () => {
    syncControlState();
    await reloadChats();
  });

  userIdInput.addEventListener("change", async () => {
    syncControlState();
    startDraftSession();
    await reloadChats();
  });

  channelInput.addEventListener("change", async () => {
    syncControlState();
    startDraftSession();
    await reloadChats();
  });

  composerForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    await sendMessage();
  });
}

function restoreSettings(): void {
  const raw = localStorage.getItem(SETTINGS_KEY);
  if (raw) {
    try {
      const parsed = JSON.parse(raw) as Partial<typeof state>;
      if (typeof parsed.apiBase === "string" && parsed.apiBase.trim() !== "") {
        state.apiBase = parsed.apiBase.trim();
      }
      if (typeof parsed.userId === "string" && parsed.userId.trim() !== "") {
        state.userId = parsed.userId.trim();
      }
      if (typeof parsed.channel === "string" && parsed.channel.trim() !== "") {
        state.channel = parsed.channel.trim();
      }
    } catch {
      localStorage.removeItem(SETTINGS_KEY);
    }
  }
  apiBaseInput.value = state.apiBase;
  userIdInput.value = state.userId;
  channelInput.value = state.channel;
}

function syncControlState(): void {
  state.apiBase = apiBaseInput.value.trim() || DEFAULT_API_BASE;
  state.userId = userIdInput.value.trim() || DEFAULT_USER_ID;
  state.channel = channelInput.value.trim() || DEFAULT_CHANNEL;
  localStorage.setItem(
    SETTINGS_KEY,
    JSON.stringify({
      apiBase: state.apiBase,
      userId: state.userId,
      channel: state.channel,
    }),
  );
}

async function reloadChats(): Promise<void> {
  try {
    const query = new URLSearchParams({
      user_id: state.userId,
      channel: state.channel,
    });
    const chats = await requestJSON<ChatSpec[]>(`/chats?${query.toString()}`);
    state.chats = chats;
    renderChatList();
  } catch (error) {
    setStatus(asErrorMessage(error), "error");
  }
}

async function openChat(chatID: string): Promise<void> {
  const chat = state.chats.find((item) => item.id === chatID);
  if (!chat) {
    setStatus(`chat not found: ${chatID}`, "error");
    return;
  }

  state.activeChatId = chat.id;
  state.activeSessionId = chat.session_id;
  renderChatHeader();
  renderChatList();

  try {
    const history = await requestJSON<ChatHistoryResponse>(`/chats/${encodeURIComponent(chat.id)}`);
    state.messages = history.messages.map(toViewMessage);
    renderMessages();
    setStatus(`Loaded ${history.messages.length} messages`, "info");
  } catch (error) {
    setStatus(asErrorMessage(error), "error");
  }
}

function startDraftSession(): void {
  state.activeChatId = null;
  state.activeSessionId = newSessionID();
  state.messages = [];
  renderChatHeader();
  renderChatList();
  renderMessages();
}

async function sendMessage(): Promise<void> {
  syncControlState();
  if (state.sending) {
    return;
  }
  const inputText = messageInput.value.trim();
  if (inputText === "") {
    setStatus("Please type a message first", "error");
    return;
  }
  if (state.apiBase === "" || state.userId === "" || state.channel === "") {
    setStatus("API base, user id and channel are required", "error");
    return;
  }

  state.sending = true;
  sendButton.disabled = true;
  const assistantID = `assistant-${Date.now()}`;

  state.messages = state.messages.concat(
    {
      id: `user-${Date.now()}`,
      role: "user",
      text: inputText,
    },
    {
      id: assistantID,
      role: "assistant",
      text: "",
    },
  );
  renderMessages();
  messageInput.value = "";
  setStatus("Streaming reply...", "info");

  try {
    await streamReply(inputText, (delta) => {
      const target = state.messages.find((item) => item.id === assistantID);
      if (!target) {
        return;
      }
      target.text += delta;
      renderMessages();
    });
    setStatus("Reply completed", "info");

    await reloadChats();
    const matched = state.chats.find(
      (chat) =>
        chat.session_id === state.activeSessionId &&
        chat.user_id === state.userId &&
        chat.channel === state.channel,
    );
    if (matched) {
      await openChat(matched.id);
    }
  } catch (error) {
    setStatus(asErrorMessage(error), "error");
  } finally {
    state.sending = false;
    sendButton.disabled = false;
  }
}

async function streamReply(userText: string, onDelta: (delta: string) => void): Promise<void> {
  const payload = {
    input: [{ role: "user", type: "message", content: [{ type: "text", text: userText }] }],
    session_id: state.activeSessionId,
    user_id: state.userId,
    channel: state.channel,
    stream: true,
  };

  const response = await fetch(toAbsoluteURL("/agent/process"), {
    method: "POST",
    headers: {
      "content-type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  if (!response.ok || !response.body) {
    const details = await safeReadError(response);
    throw new Error(details);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let doneReceived = false;

  while (!doneReceived) {
    const chunk = await reader.read();
    if (chunk.done) {
      break;
    }
    buffer += decoder.decode(chunk.value, { stream: true }).replaceAll("\r", "");
    const result = consumeSSEBuffer(buffer, onDelta);
    buffer = result.rest;
    doneReceived = result.done;
  }

  buffer += decoder.decode().replaceAll("\r", "");
  if (!doneReceived && buffer.trim() !== "") {
    const result = consumeSSEBuffer(`${buffer}\n\n`, onDelta);
    doneReceived = result.done;
  }

  if (!doneReceived) {
    throw new Error("SSE stream ended before [DONE]");
  }
}

function consumeSSEBuffer(raw: string, onDelta: (delta: string) => void): { done: boolean; rest: string } {
  let buffer = raw;
  let done = false;
  while (!done) {
    const boundary = buffer.indexOf("\n\n");
    if (boundary < 0) {
      break;
    }
    const block = buffer.slice(0, boundary);
    buffer = buffer.slice(boundary + 2);
    done = consumeSSEBlock(block, onDelta) || done;
  }
  return { done, rest: buffer };
}

function consumeSSEBlock(block: string, onDelta: (delta: string) => void): boolean {
  if (block.trim() === "") {
    return false;
  }
  const dataLines: string[] = [];
  for (const line of block.split("\n")) {
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (dataLines.length === 0) {
    return false;
  }
  const data = dataLines.join("\n");
  if (data === "[DONE]") {
    return true;
  }
  try {
    const payload = JSON.parse(data) as { delta?: unknown };
    if (typeof payload.delta === "string") {
      onDelta(payload.delta);
      return false;
    }
  } catch {
    onDelta(data);
    return false;
  }
  throw new Error("Invalid SSE payload without delta");
}

function renderChatList(): void {
  chatList.innerHTML = "";
  if (state.chats.length === 0) {
    const li = document.createElement("li");
    li.className = "message-empty";
    li.textContent = "No sessions found for current user/channel.";
    chatList.appendChild(li);
    return;
  }

  state.chats.forEach((chat, index) => {
    const li = document.createElement("li");
    li.className = "chat-list-item";
    li.style.animationDelay = `${Math.min(index * 24, 180)}ms`;

    const button = document.createElement("button");
    button.type = "button";
    button.className = "chat-item-btn";
    if (chat.id === state.activeChatId) {
      button.classList.add("active");
    }
    button.addEventListener("click", () => {
      void openChat(chat.id);
    });

    const title = document.createElement("span");
    title.className = "chat-title";
    title.textContent = chat.name || "Untitled Chat";

    const meta = document.createElement("span");
    meta.className = "chat-meta";
    meta.textContent = `Session ${chat.session_id} | Updated ${compactTime(chat.updated_at)}`;

    button.append(title, meta);
    li.appendChild(button);
    chatList.appendChild(li);
  });
}

function renderChatHeader(): void {
  const active = state.chats.find((chat) => chat.id === state.activeChatId);
  chatTitle.textContent = active ? active.name : "Draft Session";
  chatSession.textContent = state.activeSessionId;
}

function renderMessages(): void {
  messageList.innerHTML = "";
  if (state.messages.length === 0) {
    const empty = document.createElement("li");
    empty.className = "message-empty";
    empty.textContent = "No messages yet. Send your first prompt.";
    messageList.appendChild(empty);
    return;
  }

  for (const message of state.messages) {
    const item = document.createElement("li");
    item.className = `message ${message.role}`;
    item.textContent = message.text || (message.role === "assistant" ? "..." : "");
    messageList.appendChild(item);
  }
  messageList.scrollTop = messageList.scrollHeight;
}

function setStatus(message: string, tone: Tone = "neutral"): void {
  statusLine.textContent = message;
  statusLine.classList.remove("error", "info");
  if (tone === "error" || tone === "info") {
    statusLine.classList.add(tone);
  }
}

async function requestJSON<T>(path: string): Promise<T> {
  const response = await fetch(toAbsoluteURL(path), {
    headers: {
      accept: "application/json",
    },
  });

  const raw = await response.text();
  const parsed = raw.trim() === "" ? null : (JSON.parse(raw) as T | ErrorEnvelope);
  if (!response.ok) {
    const errorBody = (parsed as ErrorEnvelope | null)?.error;
    const detail = errorBody?.code ? `${errorBody.code}: ${errorBody.message ?? "request failed"}` : raw;
    throw new Error(detail || `request failed (${response.status})`);
  }
  return parsed as T;
}

async function safeReadError(response: Response): Promise<string> {
  const fallback = `request failed (${response.status})`;
  try {
    const raw = await response.text();
    const parsed = raw.trim() === "" ? null : (JSON.parse(raw) as ErrorEnvelope);
    if (parsed?.error?.code) {
      return `${parsed.error.code}: ${parsed.error.message ?? "request failed"}`;
    }
    return raw || fallback;
  } catch {
    return fallback;
  }
}

function toViewMessage(message: RuntimeMessage): ViewMessage {
  const joined = (message.content ?? [])
    .map((item) => item.text ?? "")
    .join("")
    .trim();
  return {
    id: message.id || `msg-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    role: message.role === "user" ? "user" : "assistant",
    text: joined,
  };
}

function toAbsoluteURL(path: string): string {
  const base = state.apiBase.replace(/\/+$/, "");
  return `${base}${path}`;
}

function asErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function compactTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function newSessionID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `session-${crypto.randomUUID()}`;
  }
  return `session-${Date.now()}`;
}

function mustElement<T extends HTMLElement>(id: string): T {
  const element = document.getElementById(id);
  if (!element) {
    throw new Error(`missing element: #${id}`);
  }
  return element as T;
}
