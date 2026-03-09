import { Outlet, createRootRouteWithContext, useMatches } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { Link } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { getCurrentUser, isAuthEnabled, logout } from '../lib/auth'
import type { User } from '../lib/api/types'
import { Separator } from '#/components/ui/separator'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '#/components/ui/breadcrumb'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from '#/components/ui/sidebar'
import { TooltipProvider } from '#/components/ui/tooltip'
import { AppSidebar } from '#/components/app-sidebar'
import { useTheme } from '#/hooks/use-theme'

const routeLabels: Record<string, string> = {
  '/dashboard': 'Dashboard',
  '/chat': 'Playground',
}

interface MyRouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<MyRouterContext>()({
  component: RootComponent,
})

function RootComponent() {
  const [user, setUser] = useState<User | null>(null)
  const [authEnabled, setAuthEnabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const matches = useMatches()
  const { theme, toggleTheme } = useTheme()

  useEffect(() => {
    async function loadAuth() {
      const enabled = await isAuthEnabled()
      setAuthEnabled(enabled)

      if (enabled) {
        const currentUser = await getCurrentUser()
        setUser(currentUser)
      }

      setLoading(false)
    }

    loadAuth()
  }, [])

  if (loading) {
    return <div className="flex min-h-screen items-center justify-center">Loading...</div>
  }

  const currentPath = matches[matches.length - 1]?.pathname ?? '/'
  const currentLabel = routeLabels[currentPath]

  return (
    <TooltipProvider>
      <SidebarProvider>
        <AppSidebar user={user} authEnabled={authEnabled} onLogout={logout} theme={theme} onToggleTheme={toggleTheme} />
        <SidebarInset>
          <header className="flex h-16 shrink-0 items-center gap-2 border-b border-white/5 bg-background/80 backdrop-blur-sm px-4">
            <SidebarTrigger className="-ml-1" />
            {currentLabel && (
              <>
                <Separator orientation="vertical" className="mr-2 h-4" />
                <Breadcrumb>
                  <BreadcrumbList>
                    <BreadcrumbItem className="hidden md:block">
                      <BreadcrumbLink asChild>
                        <Link to="/dashboard">Home</Link>
                      </BreadcrumbLink>
                    </BreadcrumbItem>
                    <BreadcrumbSeparator className="hidden md:block" />
                    <BreadcrumbItem>
                      <BreadcrumbPage>{currentLabel}</BreadcrumbPage>
                    </BreadcrumbItem>
                  </BreadcrumbList>
                </Breadcrumb>
              </>
            )}
          </header>

          <main className="relative flex-1">
            <Outlet />
          </main>
        </SidebarInset>
      </SidebarProvider>

      {/* Dev tools */}
      <TanStackRouterDevtools position="bottom-right" />
      <ReactQueryDevtools initialIsOpen={false} />
    </TooltipProvider>
  )
}
