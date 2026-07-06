/**
 * AIA Gateway API key, injected at build time by Vite from VITE_AIA_API_KEY.
 *
 * Set the value in .env (see .env.example) or via your shell:
 *   VITE_AIA_API_KEY=xxx bun run dev
 */
export const AIA_API_KEY: string = import.meta.env.VITE_AIA_API_KEY;
