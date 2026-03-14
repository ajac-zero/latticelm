import { Zap, LayoutDashboard, MessageSquare, LogOut, Sun, Moon, ChevronsUpDown, Users, BarChart3 } from 'lucide-react'
import { Link, useMatches } from '@tanstack/react-router'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '#/components/ui/sidebar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import { Avatar, AvatarFallback } from '#/components/ui/avatar'
import type { User } from '#/lib/api/types'

const navItems = [
  { title: 'Dashboard', to: '/dashboard' as const, icon: LayoutDashboard, adminOnly: true },
  { title: 'Users', to: '/users' as const, icon: Users, adminOnly: true },
  { title: 'Usage', to: '/usage' as const, icon: BarChart3, adminOnly: false, requiresUsage: true },
  { title: 'Playground', to: '/chat' as const, icon: MessageSquare, adminOnly: false },
]

function getInitials(email: string) {
  return email.slice(0, 2).toUpperCase()
}

export function AppSidebar({
  user,
  authEnabled,
  usageEnabled,
  onLogout,
  theme,
  onToggleTheme,
}: {
  user: User | null
  authEnabled: boolean
  usageEnabled: boolean
  onLogout: () => void
  theme: 'light' | 'dark'
  onToggleTheme: () => void
}) {
  const matches = useMatches()
  const currentPath = matches[matches.length - 1]?.pathname ?? '/'
  const { isMobile } = useSidebar()

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link to={user?.is_admin ? '/dashboard' : '/chat'}>
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg border border-primary/20 bg-gradient-to-br from-primary/20 to-primary/5">
                  <Zap className="size-4 text-primary" />
                </div>
                <span className="truncate text-lg font-medium tracking-tight">LatticeLM</span>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Navigation</SidebarGroupLabel>
          <SidebarMenu>
            {navItems.map((item) => {
              // Only show admin pages to admin users
              if (item.adminOnly && (!user || !user.is_admin)) {
                return null
              }
              // Hide usage page when usage tracking is disabled
              if (item.requiresUsage && !usageEnabled) {
                return null
              }
              return (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    asChild
                    isActive={currentPath === item.to}
                    tooltip={item.title}
                  >
                    <Link to={item.to}>
                      <item.icon />
                      <span>{item.title}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          {authEnabled && user ? (
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <SidebarMenuButton
                    size="lg"
                    className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
                  >
                    <Avatar className="h-8 w-8 rounded-lg">
                      <AvatarFallback className="rounded-lg bg-primary/20 text-xs text-primary">
                        {getInitials(user.email)}
                      </AvatarFallback>
                    </Avatar>
                    <div className="grid flex-1 text-left text-sm leading-tight">
                      <span className="truncate font-medium">{user.email}</span>
                    </div>
                    <ChevronsUpDown className="ml-auto size-4" />
                  </SidebarMenuButton>
                </DropdownMenuTrigger>
                <DropdownMenuContent
                  className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
                  side={isMobile ? 'bottom' : 'right'}
                  align="end"
                  sideOffset={4}
                >
                  <DropdownMenuLabel className="font-normal">
                    <div className="flex items-center gap-2 text-left text-sm">
                      <Avatar className="h-8 w-8 rounded-lg">
                        <AvatarFallback className="rounded-lg bg-primary/20 text-xs text-primary">
                          {getInitials(user.email)}
                        </AvatarFallback>
                      </Avatar>
                      <div className="flex flex-col">
                        <span className="truncate font-medium">{user.email}</span>
                        {user.is_admin && (
                          <span className="text-xs text-muted-foreground">Admin</span>
                        )}
                      </div>
                    </div>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={onToggleTheme}>
                    {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
                    {theme === 'dark' ? 'Light mode' : 'Dark mode'}
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={onLogout}>
                    <LogOut className="size-4" />
                    Logout
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          ) : (
            <SidebarMenuItem>
              <SidebarMenuButton tooltip={theme === 'dark' ? 'Light mode' : 'Dark mode'} onClick={onToggleTheme}>
                {theme === 'dark' ? <Sun /> : <Moon />}
                <span>{theme === 'dark' ? 'Light mode' : 'Dark mode'}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          )}
        </SidebarMenu>
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}
