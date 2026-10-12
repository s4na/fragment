import { useEffect, useRef, useState } from "preact/hooks";
import { api } from "./api";

type Message = {
  role: string;
  text: string;
  type?: string;
  key?: { kind: string; path: string };
};
type ChatState = {
  messages: Message[];
  busy: boolean;
  flow_path: string;
  version: number;
};

export function Chat({
  project,
  onSaved,
}: {
  project: string;
  onSaved: () => void;
}) {
  const [state, setState] = useState<ChatState>({
    messages: [],
    busy: false,
    flow_path: "",
    version: 0,
  });
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [connected, setConnected] = useState(false);
  const callback = useRef(onSaved);
  callback.current = onSaved;
  const last = useRef<ChatState | null>(null);
  const end = useRef<HTMLDivElement>(null);
  const route = `/api/chat?project=${encodeURIComponent(project)}`;
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const next = await api<ChatState>(route);
        if (!active) return;
        if (last.current && next.version < last.current.version) {
          timer = setTimeout(() => void poll(), 1000);
          return;
        }
        if (last.current?.busy && !next.busy) callback.current();
        last.current = next;
        setState(next);
        setConnected(true);
      } catch (e) {
        if (!active) return;
        setError(e instanceof Error ? e.message : String(e));
      }
      if (active) timer = setTimeout(() => void poll(), 1000);
    }
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [route]);
  useEffect(() => {
    end.current?.scrollIntoView({ block: "nearest" });
  }, [state.version]);
  async function send(event: Event) {
    event.preventDefault();
    setSending(true);
    setError("");
    try {
      const next = await api<ChatState>(route, "POST", { text });
      // The turn may finish before the first poll.
      if (!next.busy) callback.current();
      setState(next);
      last.current = next;
      setText("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSending(false);
    }
  }
  return (
    <aside class="chat-panel" aria-label="プロジェクトのチャット">
      <div class="chat-heading">
        <h2>チャット</h2>
        <span>{state.busy ? "応答中…" : "Codex"}</span>
      </div>
      <p class="muted">
        会話はflowに残ります。文書を保存・更新したいときは、チャットで指示してください。
      </p>
      <div
        class="chat-messages"
        role="log"
        aria-label="会話"
        aria-live="polite"
      >
        {state.messages.map((m, i) => (
          <div key={i} class={`chat-message ${m.role} ${m.type || ""}`}>
            <span class="chat-role">
              {m.role === "user"
                ? "あなた"
                : m.role === "assistant"
                  ? "Codex"
                  : "保存状況"}
            </span>
            <p>{m.text}</p>
          </div>
        ))}
        <div ref={end} />
      </div>
      {error && (
        <p class="chat-error" role="alert">
          {error}
        </p>
      )}
      <form onSubmit={(e) => void send(e)}>
        <label for="chat-input">メッセージ</label>
        <textarea
          id="chat-input"
          value={text}
          rows={4}
          maxLength={32768}
          disabled={!connected || state.busy || sending}
          placeholder="この内容をstock/概要.mdに保存して"
          onInput={(e) => setText(e.currentTarget.value)}
        />
        <button
          class="primary"
          type="submit"
          disabled={!connected || state.busy || sending || !text.trim()}
        >
          送信
        </button>
      </form>
      {state.flow_path && <p class="muted chat-path">flow/{state.flow_path}</p>}
    </aside>
  );
}
