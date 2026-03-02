import { z } from "zod";

// ============================================================
// Content Parts
// ============================================================

const outputTextContentSchema = z.object({
  type: z.literal("output_text"),
  text: z.string(),
  annotations: z.array(z.object({
    type: z.string(),
  })),
});

const inputTextContentSchema = z.object({
  type: z.literal("input_text"),
  text: z.string(),
});

const refusalContentSchema = z.object({
  type: z.literal("refusal"),
  refusal: z.string(),
});

const contentPartSchema = z.discriminatedUnion("type", [
  outputTextContentSchema,
  inputTextContentSchema,
  refusalContentSchema,
]);

// ============================================================
// Output Items
// ============================================================

const messageOutputItemSchema = z.object({
  type: z.literal("message"),
  id: z.string(),
  status: z.enum(["in_progress", "completed", "incomplete"]),
  role: z.enum(["user", "assistant", "system", "developer"]),
  content: z.array(contentPartSchema),
});

const functionCallOutputItemSchema = z.object({
  type: z.literal("function_call"),
  id: z.string(),
  call_id: z.string(),
  name: z.string(),
  arguments: z.string(),
  status: z.enum(["in_progress", "completed", "incomplete"]),
});

const outputItemSchema = z.discriminatedUnion("type", [
  messageOutputItemSchema,
  functionCallOutputItemSchema,
]);

// ============================================================
// Usage
// ============================================================

const usageSchema = z.object({
  input_tokens: z.number().int(),
  output_tokens: z.number().int(),
  total_tokens: z.number().int(),
  input_tokens_details: z.object({
    cached_tokens: z.number().int(),
  }),
  output_tokens_details: z.object({
    reasoning_tokens: z.number().int(),
  }),
});

// ============================================================
// ResponseResource
// ============================================================

export const responseResourceSchema = z.object({
  id: z.string(),
  object: z.literal("response"),
  created_at: z.number().int(),
  completed_at: z.number().int().nullable(),
  status: z.string(),
  incomplete_details: z.object({ reason: z.string() }).nullable(),
  model: z.string(),
  previous_response_id: z.string().nullable(),
  instructions: z.string().nullable(),
  output: z.array(outputItemSchema),
  error: z.object({ type: z.string(), message: z.string() }).nullable(),
  tools: z.any(),
  tool_choice: z.any(),
  truncation: z.string(),
  parallel_tool_calls: z.boolean(),
  text: z.any(),
  top_p: z.number(),
  presence_penalty: z.number(),
  frequency_penalty: z.number(),
  top_logprobs: z.number().int(),
  temperature: z.number(),
  reasoning: z.any().nullable(),
  usage: usageSchema.nullable(),
  max_output_tokens: z.number().int().nullable(),
  max_tool_calls: z.number().int().nullable(),
  store: z.boolean(),
  background: z.boolean(),
  service_tier: z.string(),
  metadata: z.any(),
  safety_identifier: z.string().nullable(),
  prompt_cache_key: z.string().nullable(),
});

export type ResponseResource = z.infer<typeof responseResourceSchema>;

// ============================================================
// Streaming Event Schemas
// ============================================================

const responseCreatedEventSchema = z.object({
  type: z.literal("response.created"),
  sequence_number: z.number().int(),
  response: responseResourceSchema,
});

const responseInProgressEventSchema = z.object({
  type: z.literal("response.in_progress"),
  sequence_number: z.number().int(),
  response: responseResourceSchema,
});

const responseCompletedEventSchema = z.object({
  type: z.literal("response.completed"),
  sequence_number: z.number().int(),
  response: responseResourceSchema,
});

const responseFailedEventSchema = z.object({
  type: z.literal("response.failed"),
  sequence_number: z.number().int(),
  response: responseResourceSchema,
});

const outputItemAddedEventSchema = z.object({
  type: z.literal("response.output_item.added"),
  sequence_number: z.number().int(),
  output_index: z.number().int(),
  item: z.object({
    id: z.string(),
    type: z.string(),
    status: z.string(),
    role: z.string().optional(),
    content: z.array(z.any()).optional(),
  }),
});

const outputItemDoneEventSchema = z.object({
  type: z.literal("response.output_item.done"),
  sequence_number: z.number().int(),
  output_index: z.number().int(),
  item: z.object({
    id: z.string(),
    type: z.string(),
    status: z.string(),
    role: z.string().optional(),
    content: z.array(z.any()).optional(),
  }),
});

const contentPartAddedEventSchema = z.object({
  type: z.literal("response.content_part.added"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  content_index: z.number().int(),
  part: z.object({
    type: z.string(),
    text: z.string().optional(),
    annotations: z.array(z.any()).optional(),
  }),
});

const contentPartDoneEventSchema = z.object({
  type: z.literal("response.content_part.done"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  content_index: z.number().int(),
  part: z.object({
    type: z.string(),
    text: z.string().optional(),
    annotations: z.array(z.any()).optional(),
  }),
});

const outputTextDeltaEventSchema = z.object({
  type: z.literal("response.output_text.delta"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  content_index: z.number().int(),
  delta: z.string(),
});

const outputTextDoneEventSchema = z.object({
  type: z.literal("response.output_text.done"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  content_index: z.number().int(),
  text: z.string(),
});

const functionCallArgsDeltaEventSchema = z.object({
  type: z.literal("response.function_call_arguments.delta"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  delta: z.string(),
});

const functionCallArgsDoneEventSchema = z.object({
  type: z.literal("response.function_call_arguments.done"),
  sequence_number: z.number().int(),
  item_id: z.string(),
  output_index: z.number().int(),
  arguments: z.string(),
});

const errorEventSchema = z.object({
  type: z.literal("error"),
  sequence_number: z.number().int(),
  error: z.object({
    type: z.string(),
    message: z.string(),
    code: z.string().nullable().optional(),
  }),
});

export const streamingEventSchema = z.discriminatedUnion("type", [
  responseCreatedEventSchema,
  responseInProgressEventSchema,
  responseCompletedEventSchema,
  responseFailedEventSchema,
  outputItemAddedEventSchema,
  outputItemDoneEventSchema,
  contentPartAddedEventSchema,
  contentPartDoneEventSchema,
  outputTextDeltaEventSchema,
  outputTextDoneEventSchema,
  functionCallArgsDeltaEventSchema,
  functionCallArgsDoneEventSchema,
  errorEventSchema,
]);

export type StreamingEvent = z.infer<typeof streamingEventSchema>;
