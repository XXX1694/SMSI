import { readTools } from "./read.js";
import { writeTools } from "./write.js";
import type { ToolDef } from "./types.js";

export const ALL_TOOLS: ToolDef[] = [...readTools, ...writeTools];
export type { ToolDef } from "./types.js";
