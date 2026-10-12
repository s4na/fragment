import { render } from "preact";
import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import "./style.css";
import { api, APIError, setCSRF } from "./api";
import { Chat } from "./chat";
import * as Icon from "./icons";
import { Palette, type PaletteAction } from "./palette";
import {
	buildTree,
	displayName,
	filterPaths,
	type Kind,
	kindInfo,
	normalizeDocName,
	readingStats,
} from "./paths";
import { Tree } from "./tree";

type Document = {
	project: string;
	kind: Kind;
	path: string;
	content: string;
	revision: string;
	html: string;
};
type Page = { paths: string[]; next?: string };
type DocumentList = {
	project: string;
	kind: Kind;
	next: string;
	loading: Promise<void> | null;
	all: boolean;
};
type Heading = { id: string; text: string; level: number };
type Toast = { id: number; text: string };

const query = (project: string, kind: Kind, path = "") =>
	new URLSearchParams({ project, kind, path }).toString();

function store(key: string, value?: string): string | null {
	try {
		if (value === undefined) return localStorage.getItem(key);
		localStorage.setItem(key, value);
	} catch {
		// 保存できない環境でも画面は動かす。
	}
	return value ?? null;
}

const isMac = /Mac|iPhone|iPad/.test(navigator.platform);
const mod = isMac ? "⌘" : "Ctrl";

function App() {
	const initial = new URLSearchParams(location.search);
	const [project, setProject] = useState(initial.get("project") || "");
	const [kind, setKind] = useState<Kind>(
		initial.get("kind") === "flow" ? "flow" : "stock",
	);
	const [projects, setProjects] = useState<string[]>([]);
	const [paths, setPaths] = useState<string[]>([]);
	const [next, setNext] = useState("");
	const [listVersion, setListVersion] = useState(0);
	const [doc, setDoc] = useState<Document | null>(null);
	const [draft, setDraft] = useState("");
	const [html, setHTML] = useState("");
	const [previewError, setPreviewError] = useState(false);
	const [conflictDraft, setConflictDraft] = useState("");
	const [latest, setLatest] = useState<Document | null>(null);
	const [editing, setEditing] = useState(false);
	const [ready, setReady] = useState(false);
	const [busy, setBusy] = useState(false);
	const [chatRefresh, setChatRefresh] = useState(false);
	const [error, setError] = useState("");
	const [root, setRoot] = useState("");
	const [modal, setModal] = useState<"project" | "document" | null>(null);
	const [name, setName] = useState("");
	const [filter, setFilter] = useState("");
	const [palette, setPalette] = useState(false);
	const [drawer, setDrawer] = useState(false);
	const [projectMenu, setProjectMenu] = useState(false);
	const [chatOpen, setChatOpen] = useState(
		store("fragment.chat") !== "closed" && window.matchMedia("(min-width: 901px)").matches,
	);
	const [chatBusy, setChatBusy] = useState(false);
	const [chatUnseen, setChatUnseen] = useState(false);
	const [dark, setDark] = useState(
		store("fragment.theme") === "dark" ||
			(store("fragment.theme") === null &&
				matchMedia("(prefers-color-scheme: dark)").matches),
	);
	const [toasts, setToasts] = useState<Toast[]>([]);
	const [headings, setHeadings] = useState<Heading[]>([]);
	const [activeHeading, setActiveHeading] = useState("");
	const ticket = useRef(0);
	const previewTicket = useRef(0);
	const documentList = useRef<DocumentList | null>(null);
	const pendingWork = useRef(0);
	const toastId = useRef(0);
	const dialog = useRef<HTMLDialogElement>(null);
	const reading = useRef<HTMLElement>(null);
	const projectMenuRef = useRef<HTMLDivElement>(null);
	const dirty = editing && doc !== null && draft !== doc.content;
	const hasProject = ready && project !== "";

	useEffect(() => {
		document.documentElement.dataset.theme = dark ? "dark" : "light";
	}, [dark]);
	useEffect(() => {
		if (modal) dialog.current?.showModal();
		else dialog.current?.close();
	}, [modal]);
	useEffect(() => {
		document.title = doc
			? `${displayName(doc.path)} · ${project} · fragment`
			: project
				? `${project} · fragment`
				: "fragment";
	}, [doc, project]);

	useEffect(() => {
		const unload = (event: BeforeUnloadEvent) => {
			if (dirty) event.preventDefault();
		};
		addEventListener("beforeunload", unload);
		return () => removeEventListener("beforeunload", unload);
	}, [dirty]);

	function toast(text: string) {
		const id = ++toastId.current;
		setToasts((previous) => [...previous, { id, text }]);
		setTimeout(
			() => setToasts((previous) => previous.filter((t) => t.id !== id)),
			3200,
		);
	}
	async function work(action: () => Promise<void>) {
		++pendingWork.current;
		setBusy(true);
		setError("");
		try {
			await action();
		} catch (e) {
			setError(e instanceof Error ? e.message : String(e));
		} finally {
			setBusy(--pendingWork.current > 0);
		}
	}
	function canLeave() {
		return !dirty || confirm("未保存の変更があります。破棄して移動しますか？");
	}
	async function list(p: string, k: Kind) {
		const previous = documentList.current;
		const state: DocumentList = {
			project: p,
			kind: k,
			next: "",
			loading: null,
			all: false,
		};
		documentList.current = state;
		let page: Page;
		try {
			page = await api<Page>(`/api/documents?${query(p, k)}&limit=100&after=`);
		} catch (e) {
			if (documentList.current !== state) return;
			documentList.current = previous;
			setListVersion((version) => version + 1);
			throw e;
		}
		if (documentList.current !== state) return;
		state.next = page.next || "";
		setPaths(page.paths);
		setNext(state.next);
		setListVersion((version) => version + 1);
	}
	async function listMore(all = false) {
		const state = documentList.current;
		if (!state?.next) return;
		if (state.loading) {
			state.all ||= all;
			return state.loading;
		}
		// 同じ一覧の取得を直列化し、切替・更新前の応答は反映しない。
		state.all = all;
		state.loading = (async () => {
			try {
				do {
					const page = await api<Page>(
						`/api/documents?${query(state.project, state.kind)}&limit=100&after=${encodeURIComponent(state.next)}`,
					);
					if (documentList.current !== state) return;
					state.next = page.next || "";
					setPaths((previous) => [...previous, ...page.paths]);
					setNext(state.next);
				} while (state.all && state.next);
			} catch (e) {
				if (documentList.current === state) throw e;
			} finally {
				state.loading = null;
			}
		})();
		return state.loading;
	}
	async function listAll() {
		await listMore(true);
	}
	useEffect(() => {
		if (next && (filter.trim() || palette)) void work(listAll);
	}, [next, filter, palette, listVersion]);
	async function requestForView<T>(id: number, route: string, method = "GET", body?: unknown) {
		try {
			const value = await api<T>(route, method, body);
			if (id === ticket.current) return value;
		} catch (e) {
			if (id === ticket.current) throw e;
		}
	}
	async function load(p: string, k: Kind, path: string) {
		const id = ++ticket.current;
		const value = await requestForView<Document>(id, `/api/document?${query(p, k, path)}`);
		if (!value) return;
		setDoc(value);
		setDraft(value.content);
		setHTML(value.html);
		setPreviewError(false);
		setEditing(false);
		setLatest(null);
		history.replaceState(null, "", `/?${query(p, k, path)}`);
		return true;
	}
	function resetDocument() {
		++ticket.current;
		documentList.current = null;
		setModal(null);
		setDoc(null);
		setDraft("");
		setHTML("");
		setEditing(false);
		setLatest(null);
		setPaths([]);
		setNext("");
		setFilter("");
	}
	function navigate(p: string, k: Kind) {
		if (!canLeave()) return;
		resetDocument();
		setProject(p);
		setKind(k);
		setProjectMenu(false);
		history.replaceState(null, "", `/?${query(p, k)}`);
		if (p) void work(async () => {
				await list(p, k);
			});
	}
	function openDoc(path: string, k: Kind = kind) {
		if (!canLeave()) return;
		setModal(null);
		setDrawer(false);
		if (k !== kind) {
			// 別の保存場所の文書へは、一覧を切り替えてから開く。
			resetDocument();
			const id = ticket.current;
			setKind(k);
			void work(async () => {
				await list(project, k);
				if (id !== ticket.current) return;
				await load(project, k, path);
			});
			return;
		}
		void work(async () => {
			await load(project, k, path);
		});
	}

	useEffect(() => {
		const id = ticket.current;
		void work(async () => {
			const token = new URLSearchParams(location.hash.slice(1)).get("token");
			if (token)
				history.replaceState(null, "", location.pathname + location.search);
			let session: { csrf: string };
			try {
				session = await api<{ csrf: string }>("/api/session");
			} catch (e) {
				if (!token) throw e;
				session = await api<{ csrf: string }>("/api/session", "POST", {
					token,
				});
			}
			setCSRF(session.csrf);
			setRoot((await api<{ data_root: string }>("/api/state")).data_root);
			const values = (await api<{ projects: string[] }>("/api/projects"))
				.projects;
			setProjects(values);
			setReady(true);
			const selected =
				project && values.includes(project) ? project : values[0] || "";
			setProject(selected);
			if (selected) {
				await list(selected, kind);
				if (id !== ticket.current) return;
				const path = initial.get("path");
				if (path && selected === project) await load(selected, kind, path);
			}
		});
	}, []);

	function refresh() {
		if (pendingWork.current > 0 || !canLeave()) return;
		const id = ticket.current;
		void work(async () => {
			const values = await requestForView<{ projects: string[] }>(id, "/api/projects");
			if (!values) return;
			setProjects(values.projects);
			if (project) await list(project, kind);
			if (id !== ticket.current) return;
			if (doc) await load(project, kind, doc.path);
			toast("最新の状態に更新しました");
		});
	}
	// 保存と競合後のリトライは同じ処理。リトライは再取得済みのリビジョンで送る。
	function put(revision: string, conflictMessage: string) {
		if (!doc) return;
		const id = ticket.current;
		void work(async () => {
			try {
				await requestForView(id, `/api/document?${query(project, kind, doc.path)}`, "PUT", {
					content: draft,
					revision,
				});
			} catch (e) {
				if (id !== ticket.current) return;
				if (e instanceof APIError && e.code === "conflict") {
					const value = await requestForView<Document>(
						id,
						`/api/document?${query(project, kind, doc.path)}`,
					);
					if (!value) return;
					setConflictDraft(draft);
					setLatest(value);
					throw new Error(conflictMessage);
				}
				throw e;
			}
			if (id !== ticket.current) return;
			await load(project, kind, doc.path);
			toast("保存しました");
		});
	}
	function save() {
		if (!doc || latest || busy || !dirty) return;
		put(
			doc.revision,
			"データが競合しました。最新データを再取得しました。下の既存の本文を確認して変更案を編集し、リトライしてください。下書きは保持しています。",
		);
	}
	function retrySave() {
		if (!latest) return;
		put(
			latest.revision,
			"再び競合しました。最新の本文を再取得しました。内容を確認して編集し直してからリトライしてください。",
		);
	}
	function closeEditor() {
		if (!doc || !canLeave()) return;
		setDraft(doc.content);
		setHTML(doc.html);
		setEditing(false);
		setLatest(null);
	}
	async function copyMarkdown() {
		if (!doc) return;
		try {
			await navigator.clipboard.writeText(doc.content);
			toast("Markdownをコピーしました");
		} catch {
			setError("クリップボードにコピーできませんでした。");
		}
	}
	useEffect(() => {
		if (!chatRefresh || busy) return;
		setChatRefresh(false);
		const id = ticket.current;
		void work(async () => {
			await list(project, kind);
			if (id !== ticket.current) return;
			if (doc && !editing) await load(project, kind, doc.path);
		});
	}, [chatRefresh, busy, project, kind]);

	// 編集中のプレビューは、入力が止まってから自動で更新する。
	useEffect(() => {
		const id = ++previewTicket.current;
		if (!editing || !doc) return;
		if (draft === doc.content) {
			setHTML(doc.html);
			setPreviewError(false);
			return;
		}
		const timer = setTimeout(async () => {
			try {
				const result = await api<{ html: string }>(
					`/api/preview?${query(project, kind, doc.path)}`,
					"POST",
					{ content: draft },
				);
				if (id !== previewTicket.current) return;
				setHTML(result.html);
				setPreviewError(false);
			} catch {
				if (id === previewTicket.current) setPreviewError(true);
			}
		}, 350);
		return () => {
			clearTimeout(timer);
			++previewTicket.current;
		};
	}, [draft, editing, doc]);

	function create(event: Event) {
		event.preventDefault();
		const id = ticket.current;
		void work(async () => {
			if (modal === "project") {
				const projectName = name.trim();
				await requestForView(id, "/api/projects", "POST", { project: projectName });
				if (id !== ticket.current) return;
				const values = await requestForView<{ projects: string[] }>(id, "/api/projects");
				if (!values) return;
				setProjects(values.projects);
				resetDocument();
				setProject(projectName);
				setKind("stock");
				history.replaceState(null, "", `/?${query(projectName, "stock")}`);
				toast(`「${projectName}」を作成しました`);
			} else {
				const path = normalizeDocName(name);
				if (!path) throw new Error("文書名を入力してください。");
				await requestForView(id, `/api/document?${query(project, kind, path)}`, "POST", {
					content: `# ${displayName(path)}\n\n`,
				});
				if (id !== ticket.current) return;
				await list(project, kind);
				if (id !== ticket.current) return;
				if (!(await load(project, kind, path))) return;
				setEditing(true);
				toast("文書を作成しました");
			}
			setModal(null);
			setName("");
		});
	}
	function startCreate(what: "project" | "document") {
		if (!canLeave()) return;
		setName("");
		setDrawer(false);
		setModal(what);
	}
	function toggleChat(open = !chatOpen) {
		setChatOpen(open);
		store("fragment.chat", open ? "open" : "closed");
		if (open) setChatUnseen(false);
	}
	function toggleTheme() {
		setDark(!dark);
		store("fragment.theme", dark ? "light" : "dark");
	}

	// 応答が終わったときに、チャットを閉じていれば通知の目印を出す。
	const wasBusy = useRef(false);
	useEffect(() => {
		if (wasBusy.current && !chatBusy && !chatOpen) setChatUnseen(true);
		wasBusy.current = chatBusy;
	}, [chatBusy]);

	// キーボードショートカット
	useEffect(() => {
		function onKey(e: KeyboardEvent) {
			const meta = e.metaKey || e.ctrlKey;
			if (meta && e.key.toLowerCase() === "k") {
				e.preventDefault();
				if (hasProject) setPalette(true);
			} else if (meta && e.key.toLowerCase() === "s" && editing) {
				e.preventDefault();
				save();
			} else if (meta && e.key.toLowerCase() === "j" && hasProject) {
				e.preventDefault();
				toggleChat();
			} else if (e.key === "Escape") {
				if (palette || modal) return;
				if (editing && !drawer && !projectMenu) {
					e.preventDefault();
					closeEditor();
				}
				setDrawer(false);
				setProjectMenu(false);
			}
		}
		addEventListener("keydown", onKey);
		return () => removeEventListener("keydown", onKey);
	});
	useEffect(() => {
		if (!projectMenu) return;
		const close = (e: MouseEvent) => {
			if (!projectMenuRef.current?.contains(e.target as Node))
				setProjectMenu(false);
		};
		addEventListener("mousedown", close);
		return () => removeEventListener("mousedown", close);
	}, [projectMenu]);

	// 表示中の文書から目次を作り、読んでいる位置を追う。
	useEffect(() => {
		const el = reading.current;
		setActiveHeading("");
		if (!el || editing || !doc) {
			setHeadings([]);
			return;
		}
		const nodes = Array.from(el.querySelectorAll<HTMLElement>("h2[id], h3[id]"));
		setHeadings(
			nodes.map((n) => ({
				id: n.id,
				text: n.textContent ?? "",
				level: Number(n.tagName[1]),
			})),
		);
		if (nodes.length === 0 || typeof IntersectionObserver === "undefined") return;
		const observer = new IntersectionObserver(
			(entries) => {
				const visible = entries.filter((e) => e.isIntersecting);
				if (visible.length > 0) setActiveHeading(visible[0].target.id);
			},
			{ rootMargin: "-72px 0px -70% 0px" },
		);
		for (const n of nodes) observer.observe(n);
		return () => observer.disconnect();
	}, [doc, editing]);

	const tree = useMemo(() => {
		const q = filter.trim();
		return buildTree(filterPaths(paths, q));
	}, [paths, filter]);
	const visibleCount = filterPaths(paths, filter).length;
	const stats = useMemo(() => readingStats(doc?.content ?? ""), [doc]);
	const actions: PaletteAction[] = useMemo(
		() => [
			{ id: "new-doc", label: "新しい文書を作成", hint: `${kind}に作成`, run: () => startCreate("document") },
			{ id: "new-project", label: "新しいプロジェクトを作成", run: () => startCreate("project") },
			{
				id: "kind",
				label: kind === "stock" ? "flow（流れる記録）に切り替え" : "stock（溜める知識）に切り替え",
				run: () => navigate(project, kind === "stock" ? "flow" : "stock"),
			},
			{ id: "chat", label: chatOpen ? "チャットを閉じる" : "チャットを開く", hint: `${mod}J`, run: () => toggleChat() },
			{ id: "theme", label: dark ? "ライトテーマにする" : "ダークテーマにする", run: toggleTheme },
			...(!busy ? [{ id: "refresh", label: "最新の状態に更新", run: refresh }] : []),
			...projects
				.filter((p) => p !== project)
				.map((p) => ({
					id: `project:${p}`,
					label: `プロジェクト「${p}」へ移動`,
					run: () => navigate(p, kind),
				})),
		],
		[kind, project, projects, chatOpen, dark, dirty, doc, busy],
	);

	const showToc = !editing && headings.length >= 2;
	const modalPath = modal === "document" ? normalizeDocName(name) : null;

	return (
		<div class="app">
			<a class="skip-link" href="#main">
				本文へ移動
			</a>
			<header class="topbar">
				<button
					type="button"
					class="icon-btn menu-btn"
					aria-label="メニューを開く"
					aria-expanded={drawer}
					aria-controls="sidebar"
					onClick={() => setDrawer(!drawer)}
				>
					<Icon.Menu />
				</button>
				<a
					class="brand"
					href="/"
					onClick={(e) => {
						if (!canLeave()) e.preventDefault();
					}}
				>
					<Icon.Logo />
					<span>fragment</span>
				</a>
				<button
					type="button"
					class="search-trigger"
					disabled={!hasProject}
					onClick={() => setPalette(true)}
				>
					<Icon.Search />
					<span>文書を検索・操作</span>
					<kbd>{mod}K</kbd>
				</button>
				<div class="topbar-actions">
					<button
						type="button"
						class="icon-btn"
						aria-label="最新の状態に更新"
						title="最新の状態に更新"
						disabled={!ready || busy}
						onClick={refresh}
					>
						<Icon.Refresh />
					</button>
					<button
						type="button"
						class="icon-btn"
						aria-label={dark ? "ライトテーマにする" : "ダークテーマにする"}
						title={dark ? "ライトテーマにする" : "ダークテーマにする"}
						onClick={toggleTheme}
					>
						{dark ? <Icon.Sun /> : <Icon.Moon />}
					</button>
					{hasProject && (
						<button
							type="button"
							class={chatOpen ? "chat-toggle active" : "chat-toggle"}
							aria-pressed={chatOpen}
							aria-controls="chat-panel"
							title={`チャットを開閉（${mod}J）`}
							onClick={() => toggleChat()}
						>
							<Icon.Chat />
							<span>チャット</span>
							{chatBusy && <span class="pulse" role="status" aria-label="応答中" />}
							{chatUnseen && <span class="badge" role="status" aria-label="新しい応答があります" />}
						</button>
					)}
				</div>
			</header>
			{error && (
				<div class="alert" role="alert">
					<Icon.Alert />
					<span>{error}</span>
					<button
						type="button"
						class="icon-btn"
						aria-label="メッセージを閉じる"
						onClick={() => setError("")}
					>
						<Icon.Close />
					</button>
				</div>
			)}
			<div class={`layout${hasProject && chatOpen ? " with-chat" : ""}`}>
				{drawer && (
					<button
						type="button"
						class="scrim"
						aria-label="メニューを閉じる"
						onClick={() => setDrawer(false)}
					/>
				)}
				<aside class={drawer ? "sidebar open" : "sidebar"} id="sidebar">
					<div class="project-switcher" ref={projectMenuRef}>
						<button
							type="button"
							class="project-button"
							aria-haspopup="listbox"
							aria-expanded={projectMenu}
							disabled={!ready || busy}
							onClick={() => setProjectMenu(!projectMenu)}
						>
							<span class="project-avatar">{(project || "?").slice(0, 1).toUpperCase()}</span>
							<span class="project-name">{project || "プロジェクトを選択"}</span>
							<Icon.Selector />
						</button>
						{projectMenu && (
							<div class="menu" role="listbox" aria-label="プロジェクト">
								{projects.map((p) => (
									<button
										type="button"
										role="option"
										aria-selected={p === project}
										class={p === project ? "menu-item selected" : "menu-item"}
										key={p}
										onClick={() => navigate(p, kind)}
									>
										{p}
										{p === project && <Icon.Check />}
									</button>
								))}
								<button
									type="button"
									class="menu-item menu-new"
									onClick={() => {
										setProjectMenu(false);
										startCreate("project");
									}}
								>
									<Icon.Plus />
									新しいプロジェクト
								</button>
							</div>
						)}
					</div>
					{/* biome-ignore lint/a11y/useSemanticElements: This group switches document locations; it contains no form fields. */}
					<div class="kinds" role="group" aria-label="文書の保存場所">
						{(["stock", "flow"] as Kind[]).map((k) => (
							<button
								type="button"
								key={k}
								class={kind === k ? "active" : ""}
								aria-pressed={kind === k}
								disabled={!project || busy}
								onClick={() => navigate(project, k)}
							>
								<strong class="kind-name">{kindInfo[k].label}</strong>
								<small class="kind-hint">{kindInfo[k].hint}</small>
							</button>
						))}
					</div>
					<div class="files-head">
						<div class="filter">
							<Icon.Search />
							<label class="sr-only" for="filter">
								文書を絞り込む
							</label>
							<input
								id="filter"
								type="search"
								placeholder="絞り込み"
								value={filter}
								disabled={!project}
								onInput={(e) => {
									setFilter(e.currentTarget.value);
									if (e.currentTarget.value && next) void work(listAll);
								}}
							/>
						</div>
						<button
							type="button"
							class="icon-btn"
							aria-label="文書を作成"
							title="新しい文書"
							disabled={!project || busy}
							onClick={() => startCreate("document")}
						>
							<Icon.Plus />
						</button>
					</div>
					<nav class="files" aria-label="文書一覧">
						<Tree
							nodes={tree}
							selected={doc?.path ?? ""}
							disabled={busy}
							forceOpen={filter.trim() !== ""}
							onOpen={openDoc}
						/>
						{next && !filter && (
							<button
								type="button"
								class="more"
								disabled={busy}
								onClick={() =>
									void work(async () => {
										await listMore();
									})
								}
							>
								さらに読み込む
							</button>
						)}
						{hasProject && !busy && paths.length === 0 && (
							<p class="muted">
								{kind}にはまだ文書がありません。
								<button type="button" class="link-btn" onClick={() => startCreate("document")}>
									最初の文書を作る
								</button>
							</p>
						)}
						{filter && visibleCount === 0 && paths.length > 0 && (
							<p class="muted">「{filter}」に一致する文書はありません。</p>
						)}
					</nav>
					<div class="storage" title={root}>
						<span>保存先</span>
						<code>{root || "接続待ち"}</code>
					</div>
				</aside>
				<main id="main" tabIndex={-1}>
					{doc ? (
						<>
							<div class="docbar">
								<div class="docbar-main">
									<nav class="breadcrumb" aria-label="文書の場所">
										<span>{project}</span>
										<span class="sep">/</span>
										<span>{kind}</span>
										{doc.path.split("/").map((part, i, all) => (
											<>
												<span class="sep">/</span>
												{i === all.length - 1 ? (
													<strong aria-current="page">{part}</strong>
												) : (
													<span>{part}</span>
												)}
											</>
										))}
									</nav>
									<p class="docmeta">
										{stats.chars.toLocaleString()}文字 · 約{stats.minutes}分
										{dirty && <span class="unsaved">● 未保存の変更</span>}
									</p>
								</div>
								<div class="actions">
									{editing ? (
										<>
											<button type="button" disabled={busy} onClick={closeEditor}>
												閉じる
											</button>
											<button
												type="button"
												class="primary"
												disabled={busy || latest !== null || !dirty}
												title={`保存（${mod}S）`}
												onClick={save}
											>
												保存
												<kbd>{mod}S</kbd>
											</button>
										</>
									) : (
										<>
											<button type="button" class="icon-btn" aria-label="Markdownをコピー" title="Markdownをコピー" onClick={() => void copyMarkdown()}>
												<Icon.Copy />
											</button>
											<button type="button" disabled={busy} onClick={() => setEditing(true)}>
												<Icon.Edit />
												編集
											</button>
										</>
									)}
								</div>
							</div>
							{editing ? (
								<>
									{latest && (
										<section class="conflict-review" aria-labelledby="conflict-title">
											<div class="conflict-head">
												<Icon.Alert />
												<div>
													<h2 id="conflict-title">他の場所で更新されていました</h2>
													<p>
														最新の本文と、あなたの下書きを見比べてください。下書きに既存の内容を反映して編集し直すと、リトライできます。
													</p>
												</div>
											</div>
											<div class="conflict-compare">
												<div>
													<h3>最新の本文</h3>
													<pre>{latest.content}</pre>
												</div>
												<div>
													<h3>あなたの下書き</h3>
													<pre>{conflictDraft}</pre>
												</div>
											</div>
											<button
												type="button"
												class="primary"
												disabled={busy || draft === conflictDraft}
												onClick={retrySave}
											>
												{draft === conflictDraft
													? "下書きを編集するとリトライできます"
													: "この下書きでリトライ"}
											</button>
										</section>
									)}
									<div class="editor">
										<section>
											<label for="markdown">Markdown</label>
											<textarea
												id="markdown"
												value={draft}
												spellcheck={false}
												disabled={busy}
												onInput={(e) => setDraft(e.currentTarget.value)}
											/>
										</section>
										<section aria-label="プレビュー">
											<div class="preview-heading">
												<span>プレビュー</span>
												{previewError && <span class="preview-error">更新できませんでした</span>}
											</div>
											<article
												class="markdown"
												dangerouslySetInnerHTML={{ __html: html }}
											/>
										</section>
									</div>
								</>
							) : (
								<div class={showToc ? "reading-wrap with-toc" : "reading-wrap"}>
									<article
										ref={reading}
										class="markdown reading"
										dangerouslySetInnerHTML={{ __html: doc.html }}
									/>
									{showToc && (
										<nav class="toc" aria-label="目次">
											<h2>目次</h2>
											<ul class="toc-list">
												{headings.map((h) => (
													<li key={h.id} class={`lv${h.level}`}>
														<a
															href={`#${h.id}`}
															aria-current={activeHeading === h.id ? "true" : undefined}
														>
															{h.text}
														</a>
													</li>
												))}
											</ul>
										</nav>
									)}
								</div>
							)}
						</>
					) : (
						<Welcome
							ready={ready}
							project={project}
							kind={kind}
							busy={busy}
							hasDocs={paths.length > 0}
							onCreate={() => startCreate("document")}
							onCreateProject={() => startCreate("project")}
							onSwitchKind={(k) => navigate(project, k)}
							onPalette={() => setPalette(true)}
						/>
					)}
				</main>
				{hasProject && (
					<div class="chat-slot" hidden={!chatOpen}>
						<Chat
							key={project}
							mod={mod}
							project={project}
							onSaved={() => setChatRefresh(true)}
							onBusy={setChatBusy}
							onOpenDoc={(k, p) => openDoc(p, k)}
							onClose={() => toggleChat(false)}
						/>
					</div>
				)}
			</div>
			<Palette
				open={palette}
				paths={paths}
				hasMore={next !== ""}
				actions={actions}
				onOpenDoc={openDoc}
				onClose={() => setPalette(false)}
				onNeedAll={() => {
					if (next) void work(listAll);
				}}
			/>
			<dialog
				class="modal"
				ref={dialog}
				aria-labelledby="modal-title"
				onCancel={(e) => {
					if (busy) e.preventDefault();
					else setModal(null);
				}}
			>
				{modal && (
					<form onSubmit={create}>
						<h2 id="modal-title">
							{modal === "project" ? "プロジェクトを作成" : `${kind}に文書を作成`}
						</h2>
						<label for="new-name">
							{modal === "project" ? "プロジェクト名" : "文書名（フォルダ/名前）"}
						</label>
						<input
							id="new-name"
							value={name}
							required
							autoFocus
							placeholder={modal === "project" ? "新しいプロジェクト" : "設計/概要"}
							disabled={busy}
							onInput={(e) => setName(e.currentTarget.value)}
						/>
						<p class="muted" aria-live="polite">
							{modal === "document"
								? modalPath
									? `${kind}/${modalPath} として作成します。`
									: "「/」でフォルダを作れます。.mdは自動で補います。"
								: "flowとstockのフォルダを作成します。"}
						</p>
						<div class="actions">
							<button type="button" disabled={busy} onClick={() => setModal(null)}>
								キャンセル
							</button>
							<button type="submit" class="primary" disabled={busy || !name.trim()}>
								作成
							</button>
						</div>
					</form>
				)}
			</dialog>
			<div class="toasts" role="status" aria-live="polite">
				{toasts.map((t) => (
					<div class="toast" key={t.id}>
						<Icon.Check />
						{t.text}
					</div>
				))}
			</div>
		</div>
	);
}

function Welcome(props: {
	ready: boolean;
	project: string;
	kind: Kind;
	busy: boolean;
	hasDocs: boolean;
	onCreate: () => void;
	onCreateProject: () => void;
	onSwitchKind: (k: Kind) => void;
	onPalette: () => void;
}) {
	const { ready, project, kind, busy, hasDocs } = props;
	if (!ready) {
		return (
			<div class="empty">
				<Icon.Logo size={52} />
				<h1>{busy ? "接続しています…" : "fragmentへ接続"}</h1>
				<p>
					{busy
						? "少しお待ちください。"
						: "サーバー起動時に表示されたURLから開いてください。"}
				</p>
			</div>
		);
	}
	if (!project) {
		return (
			<div class="empty">
				<Icon.Logo size={52} />
				<h1>はじめましょう</h1>
				<p>まずはプロジェクトを作り、考えや決定を残す場所を用意します。</p>
				<button type="button" class="primary lg" onClick={props.onCreateProject}>
					<Icon.Plus />
					プロジェクトを作成
				</button>
				<Concepts />
			</div>
		);
	}
	const other: Kind = kind === "stock" ? "flow" : "stock";
	return (
		<div class="empty">
			<Icon.Logo size={52} />
			<h1>{hasDocs ? "文書を選んでください" : `${kind}はまだ空です`}</h1>
			<p>
				{hasDocs
					? "左の一覧から開くか、検索ですばやく移動できます。"
					: "文書を作るか、右のチャットで保存を頼んでみましょう。"}
			</p>
			<div class="empty-actions">
				<button type="button" class="primary lg" disabled={busy} onClick={props.onCreate}>
					<Icon.Plus />
					文書を作成
				</button>
				{hasDocs && (
					<button type="button" class="lg" onClick={props.onPalette}>
						<Icon.Search />
						検索 <kbd>{mod}K</kbd>
					</button>
				)}
				<button type="button" class="lg" disabled={busy} onClick={() => props.onSwitchKind(other)}>
					{other}を見る
				</button>
			</div>
			{!hasDocs && <Concepts />}
		</div>
	);
}

function Concepts() {
	return (
		<dl class="concepts">
			<div>
				<dt>flow</dt>
				<dd>チャットやメモなど、流れていく記録。会話は自動で残ります。</dd>
			</div>
			<div>
				<dt>stock</dt>
				<dd>整理して溜めていく知識。保存を指示した内容がここに蓄積されます。</dd>
			</div>
		</dl>
	);
}

const rootElement = document.getElementById("app");
if (!rootElement) throw new Error("画面の表示先が見つかりません");
render(<App />, rootElement);
