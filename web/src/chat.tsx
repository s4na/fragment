import { useEffect, useRef, useState } from "preact/hooks";
import { api } from "./api";
import * as Icon from "./icons";
import { displayName } from "./paths";

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

const suggestions = [
	"この内容をstock/概要.mdに保存して",
	"既存の説明を保って、この決定事項を追記して",
	"stockの文書を読んで、抜けている点を教えて",
];

export function Chat({
	project,
	onSaved,
	onBusy,
	onOpenDoc,
	onClose,
}: {
	project: string;
	onSaved: () => void;
	onBusy: (busy: boolean) => void;
	onOpenDoc: (kind: "flow" | "stock", path: string) => void;
	onClose: () => void;
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
	const busyCallback = useRef(onBusy);
	busyCallback.current = onBusy;
	const last = useRef<ChatState | null>(null);
	const end = useRef<HTMLDivElement>(null);
	const input = useRef<HTMLTextAreaElement>(null);
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
				setError("");
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
		busyCallback.current(state.busy);
	}, [state.busy]);
	useEffect(() => {
		end.current?.scrollIntoView({ block: "nearest" });
	}, [state.version]);
	// 入力量に合わせて入力欄の高さを伸ばす。
	useEffect(() => {
		const el = input.current;
		if (!el) return;
		// 空のときはプレースホルダの折り返しで高さが膨らまないよう、CSSの既定に任せる。
		el.style.height = "";
		if (text) el.style.height = `${Math.min(el.scrollHeight, 200)}px`;
	}, [text]);

	async function send(event?: Event) {
		event?.preventDefault();
		if (!text.trim() || state.busy || sending || !connected) return;
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
	const disabled = !connected || state.busy || sending;
	return (
		<aside class="chat-panel" id="chat-panel" aria-label="プロジェクトのチャット">
			<header class="chat-heading">
				<div>
					<h2>チャット</h2>
					<p class={state.busy ? "chat-status busy" : "chat-status"}>
						<span class="dot" />
						{state.busy ? "Codexが応答中…" : connected ? "Codex・待機中" : "接続中…"}
					</p>
				</div>
				<button
					type="button"
					class="icon-btn"
					aria-label="チャットを閉じる"
					onClick={onClose}
				>
					<Icon.Close />
				</button>
			</header>
			<div class="chat-messages" role="log" aria-label="会話" aria-live="polite">
				{connected && state.messages.length === 0 && (
					<div class="chat-empty">
						<p>
							考えたことを話しかけるだけで、整理して<strong>stock</strong>
							に残します。会話そのものは<strong>flow</strong>に保存されます。
						</p>
						<ul class="chips">
							{suggestions.map((s) => (
								<li key={s}>
									<button
										type="button"
										class="chip"
										onClick={() => {
											setText(s);
											input.current?.focus();
										}}
									>
										{s}
									</button>
								</li>
							))}
						</ul>
					</div>
				)}
				{state.messages.map((m, i) =>
					m.role === "user" || m.role === "assistant" ? (
						<div key={i} class={`chat-message ${m.role}`}>
							<span class="chat-role">
								{m.role === "user" ? "あなた" : "Codex"}
							</span>
							<p>{m.text}</p>
						</div>
					) : (
						<div key={i} class={`chat-event ${m.type || ""}`}>
							<span>{m.text}</span>
							{m.key && (
								<button
									type="button"
									class="link-btn"
									onClick={() =>
										onOpenDoc(m.key?.kind === "flow" ? "flow" : "stock", m.key?.path ?? "")
									}
								>
									{displayName(m.key.path)} を開く
								</button>
							)}
						</div>
					),
				)}
				<div ref={end} />
			</div>
			{error && (
				<p class="chat-error" role="alert">
					<Icon.Alert />
					{error}
				</p>
			)}
			<form class="chat-form" onSubmit={(e) => void send(e)}>
				<label class="sr-only" for="chat-input">
					メッセージ
				</label>
				<textarea
					id="chat-input"
					ref={input}
					value={text}
					rows={2}
					maxLength={32768}
					disabled={disabled}
					placeholder="Codexに相談・保存を指示…"
					onInput={(e) => setText(e.currentTarget.value)}
					onKeyDown={(e) => {
						// 日本語入力の確定Enterで誤送信しないよう、⌘/Ctrl+Enterで送る。
						if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && !e.isComposing) {
							e.preventDefault();
							void send();
						}
					}}
				/>
				<div class="chat-form-foot">
					<span class="muted">
						<kbd>⌘</kbd>
						<kbd>Enter</kbd> で送信
						{state.flow_path && <> · flow/{state.flow_path}</>}
					</span>
					<button class="primary" type="submit" disabled={disabled || !text.trim()}>
						<Icon.Send />
						送信
					</button>
				</div>
			</form>
		</aside>
	);
}
