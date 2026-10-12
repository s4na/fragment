import { render } from "preact";
import { useEffect, useRef, useState } from "preact/hooks";
import "./style.css";
import { api, APIError, setCSRF } from "./api";
import { Chat } from "./chat";

type Kind = "flow" | "stock";
type Document = {
  project: string;
  kind: Kind;
  path: string;
  content: string;
  revision: string;
  html: string;
};
type Page = { paths: string[]; next?: string };
const query = (project: string, kind: Kind, path = "") =>
  new URLSearchParams({ project, kind, path }).toString();

function App() {
  const initial = new URLSearchParams(location.search);
  const [project, setProject] = useState(initial.get("project") || "");
  const [kind, setKind] = useState<Kind>(
    initial.get("kind") === "flow" ? "flow" : "stock",
  );
  const [projects, setProjects] = useState<string[]>([]);
  const [paths, setPaths] = useState<string[]>([]);
  const [next, setNext] = useState("");
  const [doc, setDoc] = useState<Document | null>(null);
  const [draft, setDraft] = useState("");
  const [html, setHTML] = useState("");
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
  const ticket = useRef(0);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (modal) dialog.current?.showModal();
    else dialog.current?.close();
  }, [modal]);
  const dirty = editing && doc !== null && draft !== doc.content;

  useEffect(() => {
    const unload = (event: BeforeUnloadEvent) => {
      if (dirty) event.preventDefault();
    };
    addEventListener("beforeunload", unload);
    return () => removeEventListener("beforeunload", unload);
  }, [dirty]);

  async function work(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }
  function canLeave() {
    return !dirty || confirm("未保存の変更があります。破棄して移動しますか？");
  }
  async function list(p: string, k: Kind, after = "") {
    const page = await api<Page>(
      `/api/documents?${query(p, k)}&limit=100&after=${encodeURIComponent(after)}`,
    );
    setPaths((previous) => (after ? [...previous, ...page.paths] : page.paths));
    setNext(page.next || "");
  }
  async function load(p: string, k: Kind, path: string) {
    const id = ++ticket.current;
    const value = await api<Document>(`/api/document?${query(p, k, path)}`);
    if (id !== ticket.current) return;
    setDoc(value);
    setDraft(value.content);
    setHTML(value.html);
    setEditing(false);
    setLatest(null);
    history.replaceState(null, "", `/?${query(p, k, path)}`);
  }
  function navigate(p: string, k: Kind) {
    if (!canLeave()) return;
    ++ticket.current;
    setProject(p);
    setKind(k);
    setDoc(null);
    setDraft("");
    setHTML("");
    setEditing(false);
    setLatest(null);
    setPaths([]);
    setNext("");
    history.replaceState(null, "", `/?${query(p, k)}`);
    if (p) void work(() => list(p, k));
  }

  useEffect(() => {
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
        const path = initial.get("path");
        if (path && selected === project) await load(selected, kind, path);
      }
    });
  }, []);

  function refresh() {
    if (!canLeave()) return;
    void work(async () => {
      setProjects(
        (await api<{ projects: string[] }>("/api/projects")).projects,
      );
      if (project) await list(project, kind);
      if (doc) await load(project, kind, doc.path);
    });
  }
  function save() {
    if (!doc) return;
    void work(async () => {
      try {
        await api(`/api/document?${query(project, kind, doc.path)}`, "PUT", {
          content: draft,
          revision: doc.revision,
        });
        await load(project, kind, doc.path);
        setLatest(null);
      } catch (e) {
        if (e instanceof APIError && e.code === "conflict") {
          setConflictDraft(draft);
          setLatest(
            await api<Document>(
              `/api/document?${query(project, kind, doc.path)}`,
            ),
          );
          throw new Error(
            "データが競合しました。最新データを再取得しました。下の既存の本文を確認して変更案を編集し、リトライしてください。下書きは保持しています。",
          );
        }
        throw e;
      }
    });
  }
  function retrySave() {
    if (!doc || !latest) return;
    void work(async () => {
      try {
        await api(`/api/document?${query(project, kind, doc.path)}`, "PUT", {
          content: draft,
          revision: latest.revision,
        });
        await load(project, kind, doc.path);
        setLatest(null);
      } catch (e) {
        if (e instanceof APIError && e.code === "conflict") {
          setConflictDraft(draft);
          setLatest(
            await api<Document>(
              `/api/document?${query(project, kind, doc.path)}`,
            ),
          );
          throw new Error(
            "再び競合しました。最新の本文を再取得しました。内容を確認して編集し直してからリトライしてください。",
          );
        }
        throw e;
      }
    });
  }
  function chatSaved() {
    setChatRefresh(true);
  }
  useEffect(() => {
    if (!chatRefresh || busy) return;
    setChatRefresh(false);
    void work(async () => {
      await list(project, kind);
      if (doc && !editing) await load(project, kind, doc.path);
    });
  }, [chatRefresh, busy, project, kind]);
  function create(event: Event) {
    event.preventDefault();
    void work(async () => {
      if (modal === "project") {
        await api("/api/projects", "POST", { project: name });
        setProjects(
          (await api<{ projects: string[] }>("/api/projects")).projects,
        );
        setProject(name);
        setKind("stock");
        setDoc(null);
        setEditing(false);
        setLatest(null);
        setPaths([]);
        setNext("");
        history.replaceState(null, "", `/?${query(name, "stock")}`);
      } else {
        await api(`/api/document?${query(project, kind, name)}`, "POST", {
          content: "",
        });
        await list(project, kind);
        await load(project, kind, name);
        setEditing(true);
      }
      setModal(null);
      setName("");
    });
  }

  return (
    <div class="app">
      <header class="topbar">
        <a
          class="brand"
          href="/"
          onClick={(e) => {
            if (!canLeave()) e.preventDefault();
          }}
        >
          <span class="brand-icon">f</span>fragment
        </a>
        <span class="topbar-note">考えを残す、文書を育てる。</span>
        <button type="button" disabled={!ready || busy} onClick={refresh}>
          ↻ 更新
        </button>
      </header>
      {error && (
        <div class="alert" role="alert">
          {error}
          <button
            type="button"
            aria-label="メッセージを閉じる"
            onClick={() => setError("")}
          >
            ×
          </button>
        </div>
      )}
      <div class={ready && project ? "layout with-chat" : "layout"}>
        <aside class="sidebar">
          <div class="section-label">
            PROJECTS
            <button
              type="button"
              aria-label="プロジェクトを作成"
              disabled={!ready || busy}
              onClick={() => {
                if (canLeave()) {
                  setName("");
                  setModal("project");
                }
              }}
            >
              ＋
            </button>
          </div>
          <label class="sr-only" for="project">
            プロジェクト
          </label>
          <select
            id="project"
            value={project}
            disabled={!ready || busy}
            onChange={(e) => navigate(e.currentTarget.value, kind)}
          >
            <option value="">プロジェクトを選択</option>
            {projects.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
          {/* biome-ignore lint/a11y/useSemanticElements: This group switches document locations; it contains no form fields. */}
          <div class="kinds" role="group" aria-label="文書の保存場所">
            {(["flow", "stock"] as Kind[]).map((k) => (
              <button
                type="button"
                key={k}
                class={kind === k ? "active" : ""}
                aria-pressed={kind === k}
                disabled={!project || busy}
                onClick={() => navigate(project, k)}
              >
                {k}
              </button>
            ))}
          </div>
          <div class="section-label">
            DOCUMENTS
            <button
              type="button"
              aria-label="文書を作成"
              disabled={!project || busy}
              onClick={() => {
                if (canLeave()) {
                  setName("");
                  setModal("document");
                }
              }}
            >
              ＋
            </button>
          </div>
          <nav class="files" aria-label="文書一覧">
            {paths.map((p) => (
              <button
                type="button"
                key={p}
                class={doc?.path === p ? "file selected" : "file"}
                disabled={busy}
                onClick={() => {
                  if (canLeave()) void work(() => load(project, kind, p));
                }}
              >
                <span class="file-icon">▤</span>
                <span>{p}</span>
              </button>
            ))}
          </nav>
          {next && (
            <button
              type="button"
              disabled={busy}
              onClick={() => void work(() => list(project, kind, next))}
            >
              もっと見る
            </button>
          )}
          {ready && project && paths.length === 0 && !busy && (
            <p class="muted">文書はまだありません。</p>
          )}
          <div class="storage">
            <span class="section-label">LOCAL STORAGE</span>
            <p title={root}>{root || "接続待ち"}</p>
          </div>
        </aside>
        <main>
          {doc ? (
            <>
              <div class="docbar">
                <div class="breadcrumb">
                  {project}
                  <span>/</span>
                  {kind}
                  <span>/</span>
                  <strong>{doc.path}</strong>
                </div>
                <div class="actions">
                  {dirty && <span class="unsaved">未保存</span>}
                  {editing ? (
                    <>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => {
                          if (canLeave()) {
                            setDraft(doc.content);
                            setHTML(doc.html);
                            setEditing(false);
                            setLatest(null);
                          }
                        }}
                      >
                        編集を閉じる
                      </button>
                      <button
                        type="button"
                        class="primary"
                        disabled={busy || latest !== null}
                        onClick={save}
                      >
                        保存
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => setEditing(true)}
                    >
                      編集
                    </button>
                  )}
                </div>
              </div>
              {editing ? (
                <>
                  {latest && (
                    <section
                      class="conflict-review"
                      aria-label="競合した最新データ"
                    >
                      <h2>最新データを確認してリトライ</h2>
                      <p>
                        既存の内容を確認して下書きに反映してください。リトライには、この取得済みデータのリビジョンを使います。
                      </p>
                      <pre>{latest.content}</pre>
                      <button
                        type="button"
                        disabled={busy || draft === conflictDraft}
                        onClick={retrySave}
                      >
                        既存の内容を確認して編集した下書きをリトライ
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
                    <section>
                      <div class="preview-heading">
                        <span>プレビュー</span>
                        <button
                          type="button"
                          disabled={busy}
                          onClick={() =>
                            void work(async () => {
                              setHTML(
                                (
                                  await api<{ html: string }>(
                                    `/api/preview?${query(project, kind, doc.path)}`,
                                    "POST",
                                    { content: draft },
                                  )
                                ).html,
                              );
                            })
                          }
                        >
                          プレビュー更新
                        </button>
                      </div>
                      <article
                        class="markdown"
                        dangerouslySetInnerHTML={{ __html: html }}
                      />
                    </section>
                  </div>
                </>
              ) : (
                <article
                  class="markdown reading"
                  dangerouslySetInnerHTML={{ __html: doc.html }}
                />
              )}
            </>
          ) : (
            <div class="empty">
              <span class="empty-icon">▤</span>
              <h1>{ready ? "文書を開く" : "fragmentへ接続"}</h1>
              <p>
                {ready
                  ? "プロジェクトを選び、flowやstockに文書を残しましょう。"
                  : "サーバー起動時に表示されたURLから接続してください。"}
              </p>
              {ready && project && (
                <button
                  type="button"
                  class="primary"
                  disabled={busy}
                  onClick={() => {
                    setName("");
                    setModal("document");
                  }}
                >
                  文書を作成
                </button>
              )}
            </div>
          )}
        </main>
        {ready && project && (
          <Chat key={project} project={project} onSaved={chatSaved} />
        )}
      </div>
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
              {modal === "project"
                ? "プロジェクトを作成"
                : `${kind}に文書を作成`}
            </h2>
            <label for="new-name">
              {modal === "project" ? "プロジェクト名" : "ファイル名・相対パス"}
            </label>
            <input
              id="new-name"
              value={name}
              required
              autoFocus
              placeholder={
                modal === "project" ? "新しいプロジェクト" : "概要.md"
              }
              disabled={busy}
              onInput={(e) => setName(e.currentTarget.value)}
            />
            <p class="muted">
              {modal === "document"
                ? "フォルダ名を含められます。本文の構成は自由です。"
                : "flowとstockのフォルダを作成します。"}
            </p>
            <div class="actions">
              <button
                type="button"
                disabled={busy}
                onClick={() => setModal(null)}
              >
                キャンセル
              </button>
              <button type="submit" class="primary" disabled={busy || !name}>
                作成
              </button>
            </div>
          </form>
        )}
      </dialog>
    </div>
  );
}
const rootElement = document.getElementById("app");
if (!rootElement) throw new Error("画面の表示先が見つかりません");
render(<App />, rootElement);
