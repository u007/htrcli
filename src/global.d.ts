/// <reference types="vite/client" />

declare const __APP_VERSION__: string;

interface ImportMetaEnv {
	readonly VITE_AIA_API_KEY: string;
}

interface ImportMeta {
	readonly env: ImportMetaEnv;
}
