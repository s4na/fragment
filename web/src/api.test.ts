import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, APIError, setCSRF } from "./api";

const fetchMock = vi.fn<typeof fetch>();

beforeEach(() => {
	fetchMock.mockReset();
	vi.stubGlobal("fetch", fetchMock);
	setCSRF("session-token");
});

afterEach(() => {
	vi.unstubAllGlobals();
	setCSRF("");
});

describe("Web UIのAPI通信", () => {
	it("文書のMarkdownと表示用HTMLを受け取れる", async () => {
		const document = {
			content: "# 概要",
			html: "<h1>概要</h1>",
			revision: "v1",
		};
		fetchMock.mockResolvedValue(Response.json(document));

		expect(await api("/api/document?project=p&kind=stock&path=a.md")).toEqual(
			document,
		);
		expect(fetchMock).toHaveBeenCalledWith(
			"/api/document?project=p&kind=stock&path=a.md",
			expect.objectContaining({ method: "GET", body: undefined }),
		);
	});

	it("文書更新でセッション・CSRF・取得済みリビジョンを送る", async () => {
		const update = { content: "# 更新", revision: "read-version" };
		fetchMock.mockResolvedValue(Response.json({ revision: "new-version" }));

		await api("/api/document", "PUT", update);

		expect(fetchMock).toHaveBeenCalledWith("/api/document", {
			method: "PUT",
			credentials: "same-origin",
			headers: {
				"Content-Type": "application/json",
				"X-Fragment-CSRF": "session-token",
			},
			body: JSON.stringify(update),
		});
	});

	it("競合時は再取得とリトライの案内を返し、古い内容を自動再送しない", async () => {
		const message =
			"最新データを再取得し、既存の内容を確認して変更案を作り直し、リトライしてください。";
		fetchMock.mockResolvedValue(
			Response.json(
				{
					error: { code: "conflict", message: "文書が更新されました" },
					message,
				},
				{ status: 409 },
			),
		);

		const request = api("/api/document", "PUT", {
			content: "stale",
			revision: "old",
		});

		await expect(request).rejects.toBeInstanceOf(APIError);
		await expect(request).rejects.toMatchObject({
			code: "conflict",
			message: `${message} (conflict)`,
		});
		expect(fetchMock).toHaveBeenCalledTimes(1);
	});

	it("サーバーのエラー理由を画面側に渡す", async () => {
		fetchMock.mockResolvedValue(
			Response.json(
				{
					error: {
						code: "invalid_argument",
						message: "文書名を確認してください",
					},
				},
				{ status: 400 },
			),
		);

		await expect(api("/api/document")).rejects.toMatchObject({
			code: "invalid_argument",
			message: "文書名を確認してください (invalid_argument)",
		});
	});

	it("エラー理由がない応答でも成功として扱わない", async () => {
		fetchMock.mockResolvedValue(Response.json({}, { status: 500 }));

		await expect(api("/api/document")).rejects.toMatchObject({
			code: "request_failed",
			message: "リクエストに失敗しました (500)",
		});
	});

	it("接続できない場合も成功として扱わない", async () => {
		const failure = new TypeError("Failed to fetch");
		fetchMock.mockRejectedValue(failure);

		await expect(api("/api/document")).rejects.toBe(failure);
	});
});
