import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import OpenAI from 'openai'
import { Settings, MessageSquare } from 'lucide-react'
import { useModels } from '#/lib/api/hooks'
import { requireAuth } from '#/lib/auth'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { Label } from '#/components/ui/label'
import { Slider } from '#/components/ui/slider'
import { Switch } from '#/components/ui/switch'
import { Textarea } from '#/components/ui/textarea'
import { Button } from '#/components/ui/button'
import {
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  ConversationScrollButton,
} from '#/components/ai-elements/conversation'
import {
  Message,
  MessageContent,
  MessageResponse,
} from '#/components/ai-elements/message'
import {
  PromptInput,
  PromptInputBody,
  PromptInputFooter,
  PromptInputTools,
  PromptInputTextarea,
  PromptInputSubmit,
  PromptInputButton,
  PromptInputSelect,
  PromptInputSelectContent,
  PromptInputSelectItem,
  PromptInputSelectTrigger,
  PromptInputSelectValue,
  type PromptInputMessage,
} from '#/components/ai-elements/prompt-input'

export const Route = createFileRoute('/chat')({
  beforeLoad: requireAuth,
  component: ChatPage,
})

interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  model?: string
}

function ChatPage() {
  const { data: models = [], isLoading: modelsLoading } = useModels()
  const [selectedModel, setSelectedModel] = useState('')
  const [instructions, setInstructions] = useState('')
  const [temperature, setTemperature] = useState(1.0)
  const [stream, setStream] = useState(true)
  const [store, setStore] = useState(true)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isLoading, setIsLoading] = useState(false)
  const [streamingText, setStreamingText] = useState('')
  const [lastResponseId, setLastResponseId] = useState<string | null>(null)

  const chatStatus = isLoading
    ? streamingText
      ? ('streaming' as const)
      : ('submitted' as const)
    : ('ready' as const)

  const client = new OpenAI({
    baseURL: `${window.location.origin}/api/v1`,
    apiKey: 'unused',
    dangerouslyAllowBrowser: true,
    fetch: (url, init) => {
      const headers = new Headers(init?.headers)
      headers.delete('Authorization')
      return fetch(url, { ...init, headers, credentials: 'include' })
    },
  })

  const clearChat = () => {
    setMessages([])
    setLastResponseId(null)
    setStreamingText('')
  }

  const handleSubmit = async ({ text }: PromptInputMessage) => {
    const trimmed = text.trim()
    if (!trimmed || isLoading) return

    const newUserMessage: ChatMessage = { role: 'user', content: trimmed }
    setMessages(prev => [...prev, newUserMessage])
    setIsLoading(true)
    setStreamingText('')

    try {
      const params: Record<string, unknown> = {
        model: selectedModel,
        temperature,
        stream,
        store,
      }

      if (store && lastResponseId) {
        params.input = trimmed
        params.previous_response_id = lastResponseId
      } else if (!store && messages.length > 0) {
        params.input = [...messages, newUserMessage].map(msg => ({
          type: 'message',
          role: msg.role,
          content: msg.content,
        }))
      } else {
        params.input = trimmed
      }

      if (instructions.trim()) {
        params.instructions = instructions.trim()
      }

      if (stream) {
        const response = await client.responses.create(params as Parameters<typeof client.responses.create>[0])
        let fullText = ''
        for await (const event of response as AsyncIterable<Record<string, unknown>>) {
          if (event.type === 'response.output_text.delta') {
            fullText += event.delta as string
            setStreamingText(fullText)
          } else if (event.type === 'response.completed') {
            if (store) {
              setLastResponseId((event.response as { id: string }).id)
            }
          }
        }
        setMessages(prev => [...prev, { role: 'assistant', content: fullText, model: selectedModel }])
      } else {
        const response = await client.responses.create(params as Parameters<typeof client.responses.create>[0]) as unknown as Record<string, unknown>
        if (store) {
          setLastResponseId(response.id as string)
        }
        const responseText = (response.output as Array<{ type: string; content: Array<{ type: string; text: string }> }>)
          ?.filter(item => item.type === 'message')
          ?.flatMap(item => item.content)
          ?.filter(part => part.type === 'output_text')
          ?.map(part => part.text)
          ?.join('') ?? ''
        setMessages(prev => [...prev, { role: 'assistant', content: responseText, model: selectedModel }])
      }
    } catch (e: unknown) {
      const message = e instanceof Error ? e.message : 'Failed to get response'
      setMessages(prev => [...prev, { role: 'assistant', content: `Error: ${message}` }])
    } finally {
      setIsLoading(false)
      setStreamingText('')
    }
  }

  return (
    <div className="absolute inset-0 flex flex-col">
      <Conversation className="flex-1">
        {messages.length === 0 && !isLoading && (
          <ConversationEmptyState
            className="absolute inset-0"
            icon={<MessageSquare className="size-8" />}
            title="Playground"
            description="Send a message to start chatting."
          />
        )}
        <ConversationContent className="px-4 py-6">

          {messages.map((msg, i) => (
            <Message key={i} from={msg.role}>
              {msg.role === 'assistant' && msg.model && (
                <span className="text-xs text-muted-foreground/60 px-1">{msg.model}</span>
              )}
              <MessageContent className={msg.role === 'user' ? 'group-[.is-user]:bg-primary group-[.is-user]:text-primary-foreground' : ''}>
                {msg.role === 'assistant' ? (
                  <MessageResponse>{msg.content}</MessageResponse>
                ) : (
                  msg.content
                )}
              </MessageContent>
            </Message>
          ))}

          {isLoading && (
            <Message from="assistant">
              <MessageContent>
                {streamingText ? (
                  <MessageResponse isAnimating>{streamingText}</MessageResponse>
                ) : (
                  <span className="inline-flex gap-0.5">
                    <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:0ms]" />
                    <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:200ms]" />
                    <span className="size-1.5 animate-bounce rounded-full bg-muted-foreground [animation-delay:400ms]" />
                  </span>
                )}
              </MessageContent>
            </Message>
          )}
        </ConversationContent>

        <ConversationScrollButton />
      </Conversation>

      <div className="border-t border-border bg-card px-4 py-4">
        <div>
          <PromptInput onSubmit={handleSubmit}>
            <PromptInputBody>
              <PromptInputTextarea placeholder="Type a message..." />
            </PromptInputBody>
            <PromptInputFooter>
              <PromptInputTools>
                <PromptInputSelect
                  value={selectedModel}
                  onValueChange={val => {
                    setSelectedModel(val)
                  }}
                  disabled={modelsLoading}
                >
                  <PromptInputSelectTrigger className="h-7 text-xs">
                    <PromptInputSelectValue
                      placeholder={modelsLoading ? 'Loading...' : 'Select model'}
                    />
                  </PromptInputSelectTrigger>
                  <PromptInputSelectContent>
                    {models.map((m: { id: string }) => (
                      <PromptInputSelectItem key={m.id} value={m.id}>
                        {m.id}
                      </PromptInputSelectItem>
                    ))}
                  </PromptInputSelectContent>
                </PromptInputSelect>

                <Popover>
                  <PopoverTrigger asChild>
                    <PromptInputButton tooltip="Settings">
                      <Settings className="size-4" />
                    </PromptInputButton>
                  </PopoverTrigger>
                  <PopoverContent className="w-80" align="start" side="top">
                    <div className="space-y-4">
                      <h3 className="text-sm font-semibold">Settings</h3>

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

                      <div className="flex items-center justify-between">
                        <Label htmlFor="stream">Stream</Label>
                        <Switch id="stream" checked={stream} onCheckedChange={setStream} />
                      </div>

                      <div className="flex items-center justify-between">
                        <Label htmlFor="store">Store Conversation</Label>
                        <Switch
                          id="store"
                          checked={store}
                          onCheckedChange={val => {
                            setStore(val)
                            if (!val) setLastResponseId(null)
                          }}
                        />
                      </div>

                      <Button onClick={clearChat} variant="destructive" className="w-full">
                        Clear Chat
                      </Button>
                    </div>
                  </PopoverContent>
                </Popover>
              </PromptInputTools>

              <PromptInputSubmit status={chatStatus} disabled={isLoading || !selectedModel} />
            </PromptInputFooter>
          </PromptInput>
        </div>
      </div>
    </div>
  )
}
