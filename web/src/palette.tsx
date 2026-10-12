import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import * as Icon from "./icons";
import { displayName, filterPaths } from "./paths";

export type PaletteAction = {
	id: string;
	label: string;
	hint?: string;
	run: () => void;
};

type Props = {
	open: boolean;
	paths: string[];
	hasMore: boolean;
	actions: PaletteAction[];
	onOpenDoc: (path: string) => void;
	onClose: () => void;
	onNeedAll: () => void;
};

export function Palette(props: Props) {
	const { open, paths, hasMore, actions, onOpenDoc, onClose, onNeedAll } = props;
	const dialog = useRef<HTMLDialogElement>(null);
	const [query, setQuery] = useState("");
	const [index, setIndex] = useState(0);
	useEffect(() => {
		const el = dialog.current;
		if (!el) return;
		if (open && !el.open) {
			setQuery("");
			setIndex(0);
			el.showModal();
			onNeedAll();
		} else if (!open && el.open) el.close();
	}, [open]);

	const items = useMemo(() => {
		const q = query.trim().toLowerCase();
		const docs = filterPaths(paths, q)
			.slice(0, 30)
			.map((p) => ({
				id: `doc:${p}`,
				group: "文書",
				label: displayName(p),
				hint: p,
				run: () => onOpenDoc(p),
			}));
		const acts = actions
			.filter((a) => !q || `${a.label} ${a.hint ?? ""}`.toLowerCase().includes(q))
			.map((a) => ({ ...a, group: "操作" }));
		// 検索語があるときは文書を先頭に、無いときは操作を先頭にする。
		return q ? [...docs, ...acts] : [...acts, ...docs];
	}, [query, paths, actions]);
	const current = Math.min(index, Math.max(items.length - 1, 0));

	function choose(i: number) {
		const item = items[i];
		if (!item) return;
		onClose();
		item.run();
	}
	function onKey(e: KeyboardEvent) {
		if (e.isComposing) return;
		if (e.key === "ArrowDown" || e.key === "ArrowUp") {
			e.preventDefault();
			const delta = e.key === "ArrowDown" ? 1 : -1;
			setIndex((current + delta + items.length) % Math.max(items.length, 1));
		} else if (e.key === "Enter") {
			e.preventDefault();
			choose(current);
		}
	}
	useEffect(() => {
		document
			.getElementById(`palette-item-${current}`)
			?.scrollIntoView({ block: "nearest" });
	}, [current]);

	let lastGroup = "";
	return (
		// biome-ignore lint/a11y/useKeyWithClickEvents: 背景クリックで閉じるのはマウス向けの補助。キーボードはEscで閉じる。
		<dialog
			ref={dialog}
			class="palette"
			aria-label="コマンドパレット"
			onClose={onClose}
			onClick={(e) => {
				if (e.target === dialog.current) onClose();
			}}
		>
			{open && (
				<div class="palette-body">
					<div class="palette-input">
						<Icon.Search />
						<input
							role="combobox"
							aria-expanded="true"
							aria-controls="palette-list"
							aria-activedescendant={`palette-item-${current}`}
							aria-label="文書を検索、または操作を実行"
							placeholder="文書を検索、または操作を実行…"
							value={query}
							autoComplete="off"
							spellcheck={false}
							onKeyDown={onKey}
							onInput={(e) => {
								setQuery(e.currentTarget.value);
								setIndex(0);
							}}
						/>
						<kbd>esc</kbd>
					</div>
					<div id="palette-list" role="listbox" class="palette-list">
						{items.map((item, i) => {
							const heading = item.group !== lastGroup;
							lastGroup = item.group;
							return (
								<div key={item.id} role="presentation">
									{heading && <div class="palette-group" role="presentation">{item.group}</div>}
									{/* biome-ignore lint/a11y/useKeyWithClickEvents: キーボード操作は検索欄のcombobox（矢印・Enter）が担う。 */}
									<div
										id={`palette-item-${i}`}
										role="option"
										tabIndex={-1}
										aria-selected={i === current}
										class={i === current ? "palette-item active" : "palette-item"}
										onMouseMove={() => setIndex(i)}
										onClick={() => choose(i)}
									>
										<span>{item.label}</span>
										{item.hint && <small>{item.hint}</small>}
									</div>
								</div>
							);
						})}
						{items.length === 0 && (
							<div class="palette-empty" role="presentation">
								「{query}」に一致する文書や操作はありません。
							</div>
						)}
					</div>
					<div class="palette-foot">
						<span>
							<kbd>↑</kbd>
							<kbd>↓</kbd> 移動
						</span>
						<span>
							<kbd>Enter</kbd> 実行
						</span>
						{hasMore && <span>文書を読み込み中…</span>}
					</div>
				</div>
			)}
		</dialog>
	);
}
