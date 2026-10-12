import type { ComponentChildren } from "preact";

function Icon({
	children,
	size = 16,
}: {
	children: ComponentChildren;
	size?: number;
}) {
	return (
		<svg
			width={size}
			height={size}
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.8"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
			focusable="false"
		>
			{children}
		</svg>
	);
}

export const Search = () => (
	<Icon>
		<circle cx="11" cy="11" r="7" />
		<path d="m20 20-3.5-3.5" />
	</Icon>
);
export const Plus = () => (
	<Icon>
		<path d="M12 5v14M5 12h14" />
	</Icon>
);
export const File = () => (
	<Icon>
		<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
		<path d="M14 3v5h5" />
	</Icon>
);
export const Chat = () => (
	<Icon>
		<path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z" />
	</Icon>
);
export const Folder = () => (
	<Icon>
		<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
	</Icon>
);
export const Chevron = () => (
	<Icon size={14}>
		<path d="m9 6 6 6-6 6" />
	</Icon>
);
export const Selector = () => (
	<Icon size={14}>
		<path d="m8 9 4-4 4 4M8 15l4 4 4-4" />
	</Icon>
);
export const Menu = () => (
	<Icon size={18}>
		<path d="M4 7h16M4 12h16M4 17h16" />
	</Icon>
);
export const Close = () => (
	<Icon>
		<path d="M6 6l12 12M18 6 6 18" />
	</Icon>
);
export const Refresh = () => (
	<Icon>
		<path d="M20 11a8 8 0 0 0-14.5-4M4 4v4h4M4 13a8 8 0 0 0 14.5 4M20 20v-4h-4" />
	</Icon>
);
export const Edit = () => (
	<Icon>
		<path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16z" />
		<path d="m14 6 4 4" />
	</Icon>
);
export const Copy = () => (
	<Icon>
		<rect x="9" y="9" width="11" height="11" rx="2" />
		<path d="M5 15V6a2 2 0 0 1 2-2h9" />
	</Icon>
);
export const Check = () => (
	<Icon>
		<path d="m5 12.5 4.5 4.5L19 7" />
	</Icon>
);
export const Send = () => (
	<Icon>
		<path d="M5 12 20 5l-4 15-3.5-6.5z" />
		<path d="m12.5 13.5 7-8.5" />
	</Icon>
);
export const Sun = () => (
	<Icon>
		<circle cx="12" cy="12" r="4" />
		<path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4" />
	</Icon>
);
export const Moon = () => (
	<Icon>
		<path d="M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z" />
	</Icon>
);
export const Alert = () => (
	<Icon>
		<path d="M12 4 2.8 19.5h18.4z" />
		<path d="M12 10v4.5M12 17.2v.1" />
	</Icon>
);

export function Logo({ size = 28 }: { size?: number }) {
	return (
		<svg
			width={size}
			height={size}
			viewBox="0 0 32 32"
			aria-hidden="true"
			focusable="false"
		>
			<rect width="32" height="32" rx="9" fill="var(--accent)" />
			<path d="M9 22V10.5L16 8v6.5l-3 1.2V22z" fill="#fff" opacity="0.95" />
			<path d="M17.5 12.8 23 11v6.2l-5.5 2z" fill="#fff" opacity="0.6" />
			<path d="M15 21.2 23 18.4V23l-8 2.2z" fill="#fff" opacity="0.8" />
		</svg>
	);
}
