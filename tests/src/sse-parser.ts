import type { z } from "zod";
import {
  streamingEventSchema,
  type StreamingEvent,
  type ResponseResource,
} from "./schemas.ts";

export interface ParsedEvent {
  event: string;
  data: unknown;
  validationResult: z.SafeParseReturnType<unknown, StreamingEvent>;
}

export interface SSEParseResult {
  events: ParsedEvent[];
  errors: string[];
  finalResponse: ResponseResource | null;
}

export async function parseSSEStream(
  response: Response,
): Promise<SSEParseResult> {
  const events: ParsedEvent[] = [];
  const errors: string[] = [];
  let finalResponse: ResponseResource | null = null;

  const reader = response.body?.getReader();
  if (!reader) {
    return { events, errors: ["No response body"], finalResponse };
  }

  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      let currentEvent = "";
      let currentData = "";

      for (const line of lines) {
        if (line.startsWith("event:")) {
          currentEvent = line.slice(6).trim();
        } else if (line.startsWith("data:")) {
          currentData = line.slice(5).trim();
        } else if (line === "" && currentData) {
          if (currentData === "[DONE]") {
            // Skip sentinel
          } else {
            try {
              const parsed = JSON.parse(currentData);
              const validationResult = streamingEventSchema.safeParse(parsed);

              events.push({
                event: currentEvent || parsed.type || "unknown",
                data: parsed,
                validationResult,
              });

              if (!validationResult.success) {
                errors.push(
                  `Event validation failed for ${parsed.type || "unknown"}: ${JSON.stringify(validationResult.error.issues)}`,
                );
              }

              if (
                parsed.type === "response.completed" ||
                parsed.type === "response.failed"
              ) {
                finalResponse = parsed.response;
              }
            } catch {
              errors.push(`Failed to parse event data: ${currentData}`);
            }
          }
          currentEvent = "";
          currentData = "";
        }
      }
    }
  } finally {
    reader.releaseLock();
  }

  return { events, errors, finalResponse };
}
