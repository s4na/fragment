export type Kind = "flow" | "stock";

export const kindInfo: Record<Kind, { label: string; hint: string }> = {
	flow: { label: "flow", hint: "流れる記録（チャットやメモ）" },
	stock: { label: "stock", hint: "溜める知識（整理した文書）" },
};

export type TreeNode =
	| { type: "dir"; name: string; path: string; children: TreeNode[] }
	| { type: "file"; name: string; path: string };

/** 文書名を正規化し、.mdが無ければ補う。空やフォルダだけの指定はnullを返す。 */
export function normalizeDocName(input: string): string | null {
	const cleaned = input
		.trim()
		.replace(/\\/g, "/")
		.split("/")
		.map((part) => part.trim())
		.filter((part) => part !== "" && part !== ".");
	if (cleaned.length === 0 || cleaned.includes("..")) return null;
	const last = cleaned[cleaned.length - 1];
	if (last === ".md") return null;
	if (!last.endsWith(".md")) cleaned[cleaned.length - 1] = `${last}.md`;
	return cleaned.join("/");
}

/** パスの一覧をフォルダ階層に変換する。フォルダが先、同種は名前順。 */
export function buildTree(paths: string[]): TreeNode[] {
	const root: TreeNode[] = [];
	for (const path of paths) {
		const parts = path.split("/");
		let level = root;
		parts.forEach((name, index) => {
			const full = parts.slice(0, index + 1).join("/");
			if (index === parts.length - 1) {
				level.push({ type: "file", name, path: full });
				return;
			}
			let dir = level.find((n) => n.type === "dir" && n.name === name);
			if (!dir) {
				dir = { type: "dir", name, path: full, children: [] };
				level.push(dir);
			}
			if (dir.type === "dir") level = dir.children;
		});
	}
	const sort = (nodes: TreeNode[]) => {
		nodes.sort((a, b) =>
			a.type !== b.type
				? a.type === "dir"
					? -1
					: 1
				: a.name.localeCompare(b.name, "ja", { numeric: true }),
		);
		for (const n of nodes) if (n.type === "dir") sort(n.children);
	};
	sort(root);
	return root;
}

const chatName = /^chats\/(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})-[0-9a-f]+\.md$/;

/** チャットの保存ファイル名（UTC）から、読みやすい日時の見出しを作る。 */
export function chatTitle(path: string): string | null {
	const m = chatName.exec(path);
	if (!m) return null;
	const date = new Date(
		Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]),
	);
	if (Number.isNaN(date.getTime())) return null;
	const p = (n: number) => String(n).padStart(2, "0");
	return `${date.getMonth() + 1}月${date.getDate()}日 ${p(date.getHours())}:${p(date.getMinutes())} のチャット`;
}

/** 一覧に出す表示名。チャットは日時、それ以外は拡張子を除いたファイル名。 */
export function displayName(path: string): string {
	return chatTitle(path) ?? (path.split("/").pop() ?? path).replace(/\.md$/, "");
}

/** 絞り込み。空白区切りのすべての語を含むパスだけを残す（大文字小文字は無視）。 */
export function filterPaths(paths: string[], query: string): string[] {
	const words = query.toLowerCase().split(/\s+/).filter(Boolean);
	if (words.length === 0) return paths;
	return paths.filter((p) => {
		const hay = `${p} ${chatTitle(p) ?? ""}`.toLowerCase();
		return words.every((w) => hay.includes(w));
	});
}

/** 文字数と、日本語・英語が混ざる文書の目安の読了分数。 */
export function readingStats(content: string): { chars: number; minutes: number } {
	const chars = content.replace(/\s/g, "").length;
	return { chars, minutes: Math.max(1, Math.round(chars / 500)) };
}
