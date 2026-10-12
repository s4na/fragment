import { useState } from "preact/hooks";
import * as Icon from "./icons";
import { displayName, type TreeNode } from "./paths";

type Props = {
	nodes: TreeNode[];
	selected: string;
	disabled: boolean;
	forceOpen: boolean;
	onOpen: (path: string) => void;
};

export function Tree({ nodes, selected, disabled, forceOpen, onOpen }: Props) {
	// 既定ではすべて開く。利用者が閉じたフォルダだけを覚える。
	const [closed, setClosed] = useState<Set<string>>(new Set());
	function toggle(path: string) {
		setClosed((previous) => {
			const next = new Set(previous);
			if (!next.delete(path)) next.add(path);
			return next;
		});
	}
	const render = (list: TreeNode[], depth: number) =>
		list.map((node) => {
			const indent = { "--depth": depth } as Record<string, number>;
			if (node.type === "dir") {
				const open = forceOpen || !closed.has(node.path);
				return (
					<li key={node.path}>
						<button
							type="button"
							class="tree-row tree-dir"
							style={indent}
							aria-expanded={open}
							onClick={() => toggle(node.path)}
						>
							<span class={open ? "chevron open" : "chevron"}>
								<Icon.Chevron />
							</span>
							<Icon.Folder />
							<span class="tree-name">{node.name}</span>
						</button>
						{open && <ul>{render(node.children, depth + 1)}</ul>}
					</li>
				);
			}
			const active = selected === node.path;
			return (
				<li key={node.path}>
					<button
						type="button"
						class={active ? "tree-row tree-file active" : "tree-row tree-file"}
						style={indent}
						aria-current={active ? "page" : undefined}
						disabled={disabled}
						title={node.path}
						onClick={() => onOpen(node.path)}
					>
						<span class="chevron-spacer" />
						<Icon.File />
						<span class="tree-name">{displayName(node.path)}</span>
					</button>
				</li>
			);
		});
	return <ul class="tree">{render(nodes, 0)}</ul>;
}
