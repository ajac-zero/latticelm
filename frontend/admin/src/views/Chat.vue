<template>
  <div class="chat-page">
    <header class="header">
      <div class="header-content">
        <router-link to="/" class="back-link">← Dashboard</router-link>
        <h1>Playground</h1>
      </div>
    </header>

    <div class="chat-container">
      <!-- Sidebar -->
      <aside class="sidebar">
        <div class="sidebar-section">
          <label class="field-label">Model</label>
          <select v-model="selectedModel" class="select-input" :disabled="modelsLoading">
            <option v-if="modelsLoading" value="">Loading...</option>
            <option v-for="m in models" :key="m.id" :value="m.id">
              {{ m.id }}
            </option>
          </select>
        </div>

        <div class="sidebar-section">
          <label class="field-label">System Instructions</label>
          <textarea
            v-model="instructions"
            class="textarea-input"
            rows="4"
            placeholder="You are a helpful assistant..."
          ></textarea>
        </div>

        <div class="sidebar-section">
          <label class="field-label">Temperature</label>
          <div class="slider-row">
            <input type="range" v-model.number="temperature" min="0" max="2" step="0.1" class="slider" />
            <span class="slider-value">{{ temperature }}</span>
          </div>
        </div>

        <div class="sidebar-section">
          <label class="field-label">Stream</label>
          <label class="toggle">
            <input type="checkbox" v-model="stream" />
            <span class="toggle-slider"></span>
          </label>
        </div>

        <button class="btn-clear" @click="clearChat">Clear Chat</button>
      </aside>

      <!-- Chat Area -->
      <main class="chat-main">
        <div class="messages" ref="messagesContainer">
          <div v-if="messages.length === 0" class="empty-chat">
            <p>Send a message to start chatting.</p>
          </div>
          <div
            v-for="(msg, i) in messages"
            :key="i"
            :class="['message', `message-${msg.role}`]"
          >
            <div class="message-role">{{ msg.role }}</div>
            <div class="message-content" v-html="renderContent(msg.content)"></div>
          </div>
          <div v-if="isLoading" class="message message-assistant">
            <div class="message-role">assistant</div>
            <div class="message-content">
              <span class="typing-indicator">
                <span></span><span></span><span></span>
              </span>
              {{ streamingText }}
            </div>
          </div>
        </div>

        <div class="input-area">
          <textarea
            v-model="userInput"
            class="chat-input"
            placeholder="Type a message..."
            rows="1"
            @keydown.enter.exact.prevent="sendMessage"
            @input="autoResize"
            ref="chatInputEl"
          ></textarea>
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
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: #f5f5f5;
}

.header {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  padding: 1rem 2rem;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.header-content {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.back-link {
  color: rgba(255, 255, 255, 0.85);
  text-decoration: none;
  font-size: 0.95rem;
}

.back-link:hover {
  color: white;
}

.header h1 {
  font-size: 1.5rem;
  font-weight: 600;
}

.chat-container {
  flex: 1;
  display: flex;
  overflow: hidden;
  height: calc(100vh - 65px);
}

/* Sidebar */
.sidebar {
  width: 280px;
  background: white;
  border-right: 1px solid #e2e8f0;
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
  overflow-y: auto;
}

.sidebar-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.field-label {
  font-size: 0.8rem;
  font-weight: 600;
  color: #4a5568;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.select-input {
  padding: 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.875rem;
  background: white;
  color: #2d3748;
}

.textarea-input {
  padding: 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  font-size: 0.875rem;
  resize: vertical;
  font-family: inherit;
  color: #2d3748;
}

.slider-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.slider {
  flex: 1;
  accent-color: #667eea;
}

.slider-value {
  font-size: 0.875rem;
  font-weight: 500;
  color: #2d3748;
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
  background-color: #cbd5e0;
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
  background-color: #667eea;
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(20px);
}

.btn-clear {
  margin-top: auto;
  padding: 0.5rem;
  background: #fed7d7;
  color: #742a2a;
  border: none;
  border-radius: 6px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
}

.btn-clear:hover {
  background: #feb2b2;
}

/* Chat Main */
.chat-main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
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
  color: #a0aec0;
  font-size: 1.1rem;
}

.message {
  max-width: 80%;
  padding: 0.75rem 1rem;
  border-radius: 12px;
  line-height: 1.5;
}

.message-user {
  align-self: flex-end;
  background: #667eea;
  color: white;
}

.message-user .message-role {
  color: rgba(255, 255, 255, 0.7);
}

.message-assistant {
  align-self: flex-start;
  background: white;
  border: 1px solid #e2e8f0;
  color: #2d3748;
}

.message-role {
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 0.25rem;
  color: #a0aec0;
}

.message-content {
  font-size: 0.95rem;
  word-break: break-word;
}

/* Typing indicator */
.typing-indicator {
  display: inline-flex;
  gap: 3px;
  margin-right: 6px;
}

.typing-indicator span {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #a0aec0;
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
  padding: 1rem 1.5rem;
  background: white;
  border-top: 1px solid #e2e8f0;
  display: flex;
  gap: 0.75rem;
  align-items: flex-end;
}

.chat-input {
  flex: 1;
  padding: 0.75rem 1rem;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  font-size: 0.95rem;
  font-family: inherit;
  resize: none;
  color: #2d3748;
  line-height: 1.4;
  max-height: 150px;
  overflow-y: auto;
}

.chat-input:focus {
  outline: none;
  border-color: #667eea;
  box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.15);
}

.btn-send {
  padding: 0.75rem 1.5rem;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 12px;
  font-size: 0.95rem;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
}

.btn-send:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-send:hover:not(:disabled) {
  opacity: 0.9;
}
</style>
