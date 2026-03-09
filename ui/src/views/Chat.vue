<template>
  <div class="chat-page">
    <div class="chat-container">
      <!-- Left Sidebar -->
      <aside class="sidebar">
        <!-- Model Selection -->
        <div class="sidebar-section">
          <label class="field-label">Model</label>
          <select v-model="selectedModel" class="select-input" :disabled="modelsLoading">
            <option v-if="modelsLoading" value="">Loading...</option>
            <option v-for="m in models" :key="m.id" :value="m.id">
              {{ m.id }}
            </option>
          </select>
        </div>

        <!-- System Instructions -->
        <div class="sidebar-section">
          <label class="field-label">System Instructions</label>
          <textarea
            v-model="instructions"
            class="textarea-input"
            rows="4"
            placeholder="You are a helpful assistant..."
          ></textarea>
        </div>

        <!-- Temperature -->
        <div class="sidebar-section">
          <div class="slider-header">
            <label class="field-label">Temperature</label>
            <span class="slider-value">{{ temperature }}</span>
          </div>
          <input type="range" v-model.number="temperature" min="0" max="2" step="0.1" class="slider" />
        </div>

        <!-- Stream Toggle -->
        <div class="sidebar-section toggle-section">
          <label class="field-label">Stream</label>
          <label class="toggle">
            <input type="checkbox" v-model="stream" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <!-- Clear Chat Button -->
        <button class="btn-clear" @click="clearChat">Clear Chat</button>
      </aside>

      <!-- Main Chat Area -->
      <main class="chat-main">
        <!-- Messages -->
        <div class="messages" ref="messagesContainer">
          <div v-if="messages.length === 0" class="empty-chat">
            <p>Send a message to start chatting.</p>
          </div>
          <div
            v-for="(msg, i) in messages"
            :key="i"
            :class="['message-wrapper', `message-${msg.role}`]"
          >
            <div class="message">
              <div v-if="msg.role === 'assistant'" class="message-role">Assistant</div>
              <div class="message-content" v-html="renderContent(msg.content)"></div>
            </div>
          </div>
          <div v-if="isLoading" class="message-wrapper message-assistant">
            <div class="message">
              <div class="message-role">Assistant</div>
              <div class="message-content">
                <span class="typing-indicator">
                  <span></span><span></span><span></span>
                </span>
                {{ streamingText }}
              </div>
            </div>
          </div>
        </div>

        <!-- Input Area -->
        <div class="input-area">
          <div class="input-container">
            <textarea
              v-model="userInput"
              class="chat-input"
              placeholder="Type a message..."
              rows="1"
              @keydown.enter.exact.prevent="sendMessage"
              @input="autoResize"
              ref="chatInputEl"
            ></textarea>
          </div>
          <button class="btn-send" @click="sendMessage" :disabled="isLoading || !userInput.trim()">
            Send
          </button>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import OpenAI from 'openai'

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

interface ModelOption {
  id: string
  provider: string
}

const models = ref<ModelOption[]>([])
const modelsLoading = ref(true)
const selectedModel = ref('')
const instructions = ref('')
const temperature = ref(1.0)
const stream = ref(true)
const userInput = ref('')
const messages = ref<ChatMessage[]>([])
const isLoading = ref(false)
const streamingText = ref('')
const lastResponseId = ref<string | null>(null)
const messagesContainer = ref<HTMLElement | null>(null)
const chatInputEl = ref<HTMLTextAreaElement | null>(null)

const client = new OpenAI({
  baseURL: `${window.location.origin}/v1`,
  apiKey: 'unused',
  dangerouslyAllowBrowser: true,
})

async function loadModels() {
  try {
    const resp = await fetch('/v1/models')
    const data = await resp.json()
    models.value = data.data || []
    if (models.value.length > 0) {
      selectedModel.value = models.value[0].id
    }
  } catch (e) {
    console.error('Failed to load models:', e)
  } finally {
    modelsLoading.value = false
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

function autoResize(e: Event) {
  const el = e.target as HTMLTextAreaElement
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 150) + 'px'
}

function renderContent(content: string): string {
  return content
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
}

function clearChat() {
  messages.value = []
  lastResponseId.value = null
  streamingText.value = ''
}

async function sendMessage() {
  const text = userInput.value.trim()
  if (!text || isLoading.value) return

  messages.value.push({ role: 'user', content: text })
  userInput.value = ''
  if (chatInputEl.value) {
    chatInputEl.value.style.height = 'auto'
  }
  scrollToBottom()

  isLoading.value = true
  streamingText.value = ''

  try {
    const params: Record<string, any> = {
      model: selectedModel.value,
      input: text,
      temperature: temperature.value,
      stream: stream.value,
    }

    if (instructions.value.trim()) {
      params.instructions = instructions.value.trim()
    }

    if (lastResponseId.value) {
      params.previous_response_id = lastResponseId.value
    }

    if (stream.value) {
      const response = await client.responses.create(params as any)

      // The SDK returns an async iterable for streaming
      let fullText = ''
      for await (const event of response as any) {
        if (event.type === 'response.output_text.delta') {
          fullText += event.delta
          streamingText.value = fullText
          scrollToBottom()
        } else if (event.type === 'response.completed') {
          lastResponseId.value = event.response.id
        }
      }

      messages.value.push({ role: 'assistant', content: fullText })
    } else {
      const response = await client.responses.create(params as any) as any
      lastResponseId.value = response.id

      const text = response.output
        ?.filter((item: any) => item.type === 'message')
        ?.flatMap((item: any) => item.content)
        ?.filter((part: any) => part.type === 'output_text')
        ?.map((part: any) => part.text)
        ?.join('') || ''

      messages.value.push({ role: 'assistant', content: text })
    }
  } catch (e: any) {
    messages.value.push({
      role: 'assistant',
      content: `Error: ${e.message || 'Failed to get response'}`,
    })
  } finally {
    isLoading.value = false
    streamingText.value = ''
    scrollToBottom()
  }
}

onMounted(() => {
  loadModels()
})
</script>

<style scoped>
.chat-page {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  background-color: var(--background);
}

/* Chat Container */
.chat-container {
  flex: 1;
  display: flex;
  overflow: hidden;
  min-height: 0;
}

/* Sidebar */
.sidebar {
  width: 18rem;
  border-right: 1px solid var(--border);
  background-color: var(--card);
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
  overflow-y: auto;
}

.sidebar-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.sidebar-section.toggle-section {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}

.field-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.6);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.select-input {
  padding: 0.5rem;
  border: 1px solid var(--border);
  border-radius: 0.375rem;
  font-size: 0.875rem;
  background-color: rgba(255, 255, 255, 0.05);
  color: var(--foreground);
  transition: background-color 0.2s;
}

.select-input:hover {
  background-color: rgba(255, 255, 255, 0.1);
}

.select-input:focus {
  outline: none;
  background-color: rgba(255, 255, 255, 0.1);
}

.textarea-input {
  padding: 0.75rem;
  border: 1px solid var(--border);
  border-radius: 0.375rem;
  font-size: 0.875rem;
  resize: vertical;
  font-family: inherit;
  background-color: rgba(255, 255, 255, 0.05);
  color: var(--foreground);
  min-height: 6rem;
  transition: background-color 0.2s;
}

.textarea-input::placeholder {
  color: rgba(255, 255, 255, 0.4);
}

.textarea-input:hover {
  background-color: rgba(255, 255, 255, 0.1);
}

.textarea-input:focus {
  outline: none;
  background-color: rgba(255, 255, 255, 0.1);
}

.slider-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.slider {
  -webkit-appearance: none;
  appearance: none;
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.1);
  outline: none;
  cursor: pointer;
}

.slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--primary);
  cursor: pointer;
}

.slider::-moz-range-thumb {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--primary);
  cursor: pointer;
  border: none;
}

.slider-value {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--foreground);
  min-width: 2rem;
  text-align: right;
}

.toggle {
  position: relative;
  width: 44px;
  height: 24px;
  cursor: pointer;
}

.toggle input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  inset: 0;
  background-color: rgba(255, 255, 255, 0.2);
  border-radius: 24px;
  transition: 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  border-radius: 50%;
  transition: 0.2s;
}

.toggle input:checked + .toggle-slider {
  background-color: var(--primary);
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(20px);
}

.btn-clear {
  margin-top: auto;
  padding: 0.5rem 1rem;
  background-color: rgba(239, 68, 68, 0.1);
  color: rgb(248, 113, 113);
  border: 1px solid rgba(239, 68, 68, 0.2);
  border-radius: 0.375rem;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.2s;
}

.btn-clear:hover {
  background-color: rgba(239, 68, 68, 0.2);
}

/* Chat Main */
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.empty-chat {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: rgba(255, 255, 255, 0.4);
  font-size: 1rem;
}

.message-wrapper {
  display: flex;
}

.message-wrapper.message-user {
  justify-content: flex-end;
}

.message-wrapper.message-assistant {
  justify-content: flex-start;
}

.message {
  max-width: 48rem;
  border-radius: 0.5rem;
  padding: 0.75rem 1rem;
}

.message-user .message {
  background-color: #6366f1;
  color: white;
}

.message-assistant .message {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border);
  color: var(--foreground);
}

.message-role {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.5rem;
  color: rgba(255, 255, 255, 0.4);
}

.message-content {
  font-size: 0.9375rem;
  line-height: 1.5;
  word-break: break-word;
}

/* Typing indicator */
.typing-indicator {
  display: inline-flex;
  gap: 3px;
  margin-right: 6px;
  vertical-align: middle;
}

.typing-indicator span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.4);
  animation: bounce 1.2s infinite;
}

.typing-indicator span:nth-child(2) { animation-delay: 0.2s; }
.typing-indicator span:nth-child(3) { animation-delay: 0.4s; }

@keyframes bounce {
  0%, 60%, 100% { transform: translateY(0); }
  30% { transform: translateY(-4px); }
}

/* Input Area */
.input-area {
  padding: 1.5rem;
  background-color: var(--card);
  border-top: 1px solid var(--border);
  display: flex;
  gap: 0.75rem;
  align-items: flex-end;
}

.input-container {
  flex: 1;
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border);
  border-radius: 0.5rem;
  padding: 0.75rem;
}

.chat-input {
  width: 100%;
  background: transparent;
  color: var(--foreground);
  border: none;
  font-size: 0.9375rem;
  font-family: inherit;
  resize: none;
  outline: none;
  line-height: 1.4;
  max-height: 150px;
  overflow-y: auto;
}

.chat-input::placeholder {
  color: rgba(255, 255, 255, 0.4);
}

.btn-send {
  padding: 0.75rem 1.5rem;
  background-color: #6366f1;
  color: white;
  border: none;
  border-radius: 0.5rem;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: background-color 0.2s;
}

.btn-send:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-send:hover:not(:disabled) {
  background-color: #5558e3;
}
</style>
