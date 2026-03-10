import { createFileRoute } from '@tanstack/react-router'
import { useState, useRef, useEffect } from 'react'
import OpenAI from 'openai'
import { Settings } from 'lucide-react'
import { useModels } from '../lib/api/hooks'
import { requireAuth } from '../lib/auth'
import { Button } from '../components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '../components/ui/popover'
import { Label } from '../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Slider } from '../components/ui/slider'
import { Switch } from '../components/ui/switch'
import { Textarea } from '../components/ui/textarea'

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
  const [store, setStore] = useState(true)
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

  // Clear lastResponseId when store is disabled
  useEffect(() => {
    if (!store) {
      setLastResponseId(null)
    }
  }, [store])

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

    const newUserMessage: ChatMessage = { role: 'user', content: text }
    setMessages(prev => [...prev, newUserMessage])
    setUserInput('')
    if (chatInputRef.current) {
      chatInputRef.current.style.height = 'auto'
    }

    setIsLoading(true)
    setStreamingText('')

    try {
      const params: Record<string, any> = {
        model: selectedModel,
        temperature: temperature,
        stream: stream,
        store: store,
      }

      // When store is false, send full conversation history in input
      // When store is true, use previous_response_id to fetch history from backend
      if (store && lastResponseId) {
        params.input = text
        params.previous_response_id = lastResponseId
      } else if (!store && messages.length > 0) {
        // Build input array with full conversation history
        const inputItems = [...messages, newUserMessage].map(msg => ({
          type: 'message',
          role: msg.role,
          content: msg.content,
        }))
        params.input = inputItems
      } else {
        // First message or store=true without history
        params.input = text
      }

      if (instructions.trim()) {
        params.instructions = instructions.trim()
      }

      if (stream) {
        const response = await client.responses.create(params as any)

        let fullText = ''
        for await (const event of response as any) {
          if (event.type === 'response.output_text.delta') {
            fullText += event.delta
            setStreamingText(fullText)
          } else if (event.type === 'response.completed') {
            if (store) {
              setLastResponseId(event.response.id)
            }
          }
        }

        setMessages(prev => [...prev, { role: 'assistant', content: fullText }])
      } else {
        const response = await client.responses.create(params as any) as any
        if (store) {
          setLastResponseId(response.id)
        }

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
    <div className="absolute inset-0 flex flex-col">
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
        <div className="flex flex-1 items-end gap-2 rounded-lg border border-input bg-background/50 px-3 py-3">
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
            className="max-h-[150px] flex-1 resize-none bg-transparent text-[15px] leading-normal outline-none placeholder:text-muted-foreground/40"
          />

          {/* Settings Popover */}
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0">
                <Settings className="h-4 w-4" />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-80" align="end">
              <div className="space-y-4">
                <h3 className="font-semibold text-sm">Settings</h3>

                {/* Model Selection */}
                <div className="space-y-2">
                  <Label htmlFor="model">Model</Label>
                  <Select value={selectedModel} onValueChange={setSelectedModel} disabled={modelsLoading}>
                    <SelectTrigger id="model">
                      <SelectValue placeholder={modelsLoading ? "Loading..." : "Select a model"} />
                    </SelectTrigger>
                    <SelectContent>
                      {models.map((m: any) => (
                        <SelectItem key={m.id} value={m.id}>
                          {m.id}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* System Instructions */}
                <div className="space-y-2">
                  <Label htmlFor="instructions">System Instructions</Label>
                  <Textarea
                    id="instructions"
                    value={instructions}
                    onChange={e => setInstructions(e.target.value)}
                    placeholder="You are a helpful assistant..."
                    rows={4}
                    className="resize-none"
                  />
                </div>

                {/* Temperature */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label htmlFor="temperature">Temperature</Label>
                    <span className="text-sm font-medium">{temperature}</span>
                  </div>
                  <Slider
                    id="temperature"
                    min={0}
                    max={2}
                    step={0.1}
                    value={[temperature]}
                    onValueChange={([value]) => setTemperature(value)}
                  />
                </div>

                {/* Stream Toggle */}
                <div className="flex items-center justify-between">
                  <Label htmlFor="stream">Stream</Label>
                  <Switch
                    id="stream"
                    checked={stream}
                    onCheckedChange={setStream}
                  />
                </div>

                {/* Store Toggle */}
                <div className="flex items-center justify-between">
                  <Label htmlFor="store">Store Conversation</Label>
                  <Switch
                    id="store"
                    checked={store}
                    onCheckedChange={setStore}
                  />
                </div>

                {/* Clear Chat Button */}
                <Button
                  onClick={clearChat}
                  variant="destructive"
                  className="w-full"
                >
                  Clear Chat
                </Button>
              </div>
            </PopoverContent>
          </Popover>
        </div>

        <Button
          onClick={sendMessage}
          disabled={isLoading || !userInput.trim()}
          className="whitespace-nowrap bg-indigo-600 hover:bg-indigo-500"
        >
          Send
        </Button>
      </div>
    </div>
  )
}
