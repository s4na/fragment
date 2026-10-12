import { describe, expect, it } from "vitest";
import {
	buildTree,
	chatTitle,
	displayName,
	filterPaths,
	normalizeDocName,
	readingStats,
} from "./paths";

describe("文書名の正規化", () => {
	it("拡張子が無ければ.mdを補い、余分な空白とスラッシュを整える", () => {
		expect(normalizeDocName("  概要 ")).toBe("概要.md");
		expect(normalizeDocName("設計//決定事項/./api")).toBe("設計/決定事項/api.md");
		expect(normalizeDocName("a.md")).toBe("a.md");
	});
	it("空・親フォルダ参照・拡張子だけの指定は拒否する", () => {
		expect(normalizeDocName("   ")).toBeNull();
		expect(normalizeDocName("../a")).toBeNull();
		expect(normalizeDocName("dir/.md")).toBeNull();
	});
});

describe("文書一覧のツリー化", () => {
	it("フォルダを先に、名前は数字を考慮して並べる", () => {
		const tree = buildTree(["b.md", "docs/10.md", "docs/2.md", "a.md"]);
		expect(tree.map((n) => n.name)).toEqual(["docs", "a.md", "b.md"]);
		const docs = tree[0];
		expect(docs.type === "dir" && docs.children.map((n) => n.name)).toEqual([
			"2.md",
			"10.md",
		]);
	});
});

describe("表示名と絞り込み", () => {
	it("チャットの保存名は読みやすい見出しにし、通常の文書は拡張子を外す", () => {
		expect(chatTitle("chats/20261012T093000-ab12.md")).toMatch(
			/^\d+月\d+日 \d{2}:\d{2} のチャット$/,
		);
		expect(chatTitle("chats/memo.md")).toBeNull();
		expect(displayName("設計/概要.md")).toBe("概要");
	});
	it("空白区切りの語をすべて含む文書だけに絞る", () => {
		const paths = ["設計/概要.md", "設計/api.md", "メモ/概要.md"];
		expect(filterPaths(paths, "設計 概要")).toEqual(["設計/概要.md"]);
		expect(filterPaths(paths, "API")).toEqual(["設計/api.md"]);
		expect(filterPaths(paths, " ")).toEqual(paths);
	});
});

describe("読了目安", () => {
	it("空白を除いた文字数から分数を出し、最低1分にする", () => {
		expect(readingStats("a b\nc")).toEqual({ chars: 3, minutes: 1 });
		expect(readingStats("あ".repeat(1500)).minutes).toBe(3);
	});
});
