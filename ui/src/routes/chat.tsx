import { createFileRoute } from '@tanstack/react-router'
import { useState, useRef, useEffect } from 'react'
import OpenAI from 'openai'
import { useModels } from '../lib/api/hooks'
import { requireAuth } from '../lib/auth'

export const Route = createFileRoute('/chat')({
  beforeLoad: requireAuth,
  component: ChatPage,
})

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

function ChatPage() {
  const { data: models = [], isLoading: modelsLoading } = useModels()
  const [selectedModel, setSelectedModel] = useState('')
  const [instructions, setInstructions] = useState('')
  const [temperature, setTemperature] = useState(1.0)
  const [stream, setStream] = useState(true)
  const [userInput, setUserInput] = useState('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [streamingText, setStreamingText] = useState('')
  const [lastResponseId, setLastResponseId] = useState<string | null>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const chatInputRef = useRef<HTMLTextAreaElement>(null)

  const client = new OpenAI({
    baseURL: `${window.location.origin}/v1`,
    apiKey: 'unused',
    dangerouslyAllowBrowser: true,
  })

  useEffect(() => {
    if (models.length > 0 && !selectedModel) {
      setSelectedModel(models[0].id)
    }
  }, [models, selectedModel])

  useEffect(() => {
    scrollToBottom()
  }, [messages, streamingText])

  const scrollToBottom = () => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTop = messagesContainerRef.current.scrollHeight
    }
  }

  const autoResize = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 150) + 'px'
  }

  const renderContent = (content: string) => {
    return content
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/\n/g, '<br>')
  }

  const clearChat = () => {
    setMessages([])
    setLastResponseId(null)
    setStreamingText('')
  }

  const sendMessage = async () => {
    const text = userInput.trim()
    if (!text || isLoading) return

    setMessages(prev => [...prev, { role: 'user', content: text }])
    setUserInput('')
    if (chatInputRef.current) {
      chatInputRef.current.style.height = 'auto'
    }

    setIsLoading(true)
    setStreamingText('')

    try {
      const params: Record<string, any> = {
        model: selectedModel,
        input: text,
        temperature: temperature,
        stream: stream,
      }

      if (instructions.trim()) {
        params.instructions = instructions.trim()
      }

      if (lastResponseId) {
        params.previous_response_id = lastResponseId
      }

      if (stream) {
        const response = await client.responses.create(params as any)

        let fullText = ''
        for await (const event of response as any) {
          if (event.type === 'response.output_text.delta') {
            fullText += event.delta
            setStreamingText(fullText)
          } else if (event.type === 'response.completed') {
            setLastResponseId(event.response.id)
          }
        }

        setMessages(prev => [...prev, { role: 'assistant', content: fullText }])
      } else {
        const response = await client.responses.create(params as any) as any
        setLastResponseId(response.id)

        const text = response.output
          ?.filter((item: any) => item.type === 'message')
          ?.flatMap((item: any) => item.content)
          ?.filter((part: any) => part.type === 'output_text')
          ?.map((part: any) => part.text)
          ?.join('') || ''

        setMessages(prev => [...prev, { role: 'assistant', content: text }])
      }
    } catch (e: any) {
      setMessages(prev => [
        ...prev,
        {
          role: 'assistant',
          content: `Error: ${e.message || 'Failed to get response'}`,
        },
      ])
    } finally {
      setIsLoading(false)
      setStreamingText('')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendMessage()
    }
  }

  return (
    <div className="absolute inset-0 flex">
      {/* Sidebar */}
      <aside className="flex w-72 flex-col gap-6 overflow-y-auto border-r border-border bg-card p-6">
        {/* Model Selection */}
        <div className="space-y-2">
          <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            Model
          </label>
          <select
            value={selectedModel}
            onChange={e => setSelectedModel(e.target.value)}
            disabled={modelsLoading}
            className="w-full rounded-md border border-input bg-background/50 px-3 py-2 text-sm transition-colors hover:bg-background/80 focus:bg-background/80 focus:outline-none"
          >
            {modelsLoading && <option value="">Loading...</option>}
            {models.map((m: any) => (
              <option key={m.id} value={m.id}>
                {m.id}
              </option>
            ))}
          </select>
        </div>

        {/* System Instructions */}
        <div className="space-y-2">
          <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            System Instructions
          </label>
          <textarea
            value={instructions}
            onChange={e => setInstructions(e.target.value)}
            rows={4}
            placeholder="You are a helpful assistant..."
            className="min-h-24 w-full resize-y rounded-md border border-input bg-background/50 px-3 py-2 text-sm transition-colors placeholder:text-muted-foreground/40 hover:bg-background/80 focus:bg-background/80 focus:outline-none"
          />
        </div>

        {/* Temperature */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Temperature
            </label>
            <span className="min-w-8 text-right text-sm font-medium">{temperature}</span>
          </div>
          <input
            type="range"
            value={temperature}
            onChange={e => setTemperature(Number(e.target.value))}
            min="0"
            max="2"
            step="0.1"
            className="h-1.5 w-full cursor-pointer appearance-none rounded-full bg-white/10 [&::-moz-range-thumb]:h-4 [&::-moz-range-thumb]:w-4 [&::-moz-range-thumb]:appearance-none [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-primary [&::-webkit-slider-thumb]:h-4 [&::-webkit-slider-thumb]:w-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary"
          />
        </div>

        {/* Stream Toggle */}
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            Stream
          </label>
          <label className="relative inline-block h-6 w-11 cursor-pointer">
            <input
              type="checkbox"
              checked={stream}
              onChange={e => setStream(e.target.checked)}
              className="peer h-0 w-0 opacity-0"
            />
            <span className="absolute inset-0 rounded-full bg-white/20 transition-colors before:absolute before:bottom-0.5 before:left-0.5 before:h-5 before:w-5 before:rounded-full before:bg-white before:transition-transform before:content-[''] peer-checked:bg-primary peer-checked:before:translate-x-5"></span>
          </label>
        </div>

        {/* Clear Chat Button */}
        <button
          onClick={clearChat}
          className="mt-auto rounded-md border border-destructive/20 bg-destructive/10 px-4 py-2 text-sm font-medium text-destructive-foreground transition-colors hover:bg-destructive/20"
        >
          Clear Chat
        </button>
      </aside>

      {/* Main Chat Area */}
      <main className="flex flex-1 flex-col">
        {/* Messages */}
        <div ref={messagesContainerRef} className="flex-1 space-y-4 overflow-y-auto p-6">
          {messages.length === 0 && (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              Send a message to start chatting.
            </div>
          )}
          {messages.map((msg, i) => (
            <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
              <div
                className={`max-w-3xl rounded-lg px-4 py-3 ${
                  msg.role === 'user'
                    ? 'bg-indigo-600 text-white'
                    : 'border border-border bg-white/5'
                }`}
              >
                {msg.role === 'assistant' && (
                  <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    Assistant
                  </div>
                )}
                <div
                  className="text-[15px] leading-relaxed"
                  dangerouslySetInnerHTML={{ __html: renderContent(msg.content) }}
                />
              </div>
            </div>
          ))}
          {isLoading && (
            <div className="flex justify-start">
              <div className="max-w-3xl rounded-lg border border-border bg-white/5 px-4 py-3">
                <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Assistant
                </div>
                <div className="text-[15px] leading-relaxed">
                  <span className="mr-1.5 inline-flex gap-0.5">
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-white/40 [animation-delay:0ms]"></span>
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-white/40 [animation-delay:200ms]"></span>
                    <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-white/40 [animation-delay:400ms]"></span>
                  </span>
                  {streamingText && (
                    <span dangerouslySetInnerHTML={{ __html: renderContent(streamingText) }} />
                  )}
                </div>
              </div>
            </div>
          )}
        </div>

        {/* Input Area */}
        <div className="flex gap-3 border-t border-border bg-card p-6">
          <div className="flex flex-1 rounded-lg border border-input bg-background/50 px-3 py-3">
            <textarea
              ref={chatInputRef}
              value={userInput}
              onChange={e => {
                setUserInput(e.target.value)
                autoResize(e)
              }}
              onKeyDown={handleKeyDown}
              placeholder="Type a message..."
              rows={1}
              className="max-h-[150px] w-full resize-none bg-transparent text-[15px] leading-normal outline-none placeholder:text-muted-foreground/40"
            />
          </div>
          <button
            onClick={sendMessage}
            disabled={isLoading || !userInput.trim()}
            className="whitespace-nowrap rounded-lg bg-indigo-600 px-6 py-3 text-[15px] font-medium text-white transition-colors hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            Send
          </button>
        </div>
      </main>
    </div>
  )
}
