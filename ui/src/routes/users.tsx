import { createFileRoute } from '@tanstack/react-router'
import { useState, useMemo } from 'react'
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  createColumnHelper,
} from '@tanstack/react-table'
import { Users as UsersIcon, Search, MoreVertical, Trash2, Shield, Ban, CheckCircle } from 'lucide-react'
import { useUsers, useUpdateUser, useDeleteUser } from '../lib/api/hooks'
import { requireAdmin } from '../lib/auth'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Badge } from '#/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '#/components/ui/table'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import type { UserDetail } from '../lib/api/types'

const columnHelper = createColumnHelper<UserDetail>()

export const Route = createFileRoute('/users')({
  beforeLoad: requireAdmin,
  component: UsersPage,
})

function UsersPage() {
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState<string>('all')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [selectedUser, setSelectedUser] = useState<UserDetail | null>(null)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  const limit = 20

  const { data, isLoading } = useUsers({
    page,
    limit,
    search: search || undefined,
    role: roleFilter !== 'all' ? roleFilter : undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
  })

  const updateUser = useUpdateUser()
  const deleteUser = useDeleteUser()

  const handleEditUser = (user: UserDetail) => {
    setSelectedUser(user)
    setEditDialogOpen(true)
  }

  const handleDeleteUser = (user: UserDetail) => {
    setSelectedUser(user)
    setDeleteDialogOpen(true)
  }

  const confirmDelete = async () => {
    if (!selectedUser) return
    try {
      await deleteUser.mutateAsync(selectedUser.id)
      setDeleteDialogOpen(false)
      setSelectedUser(null)
    } catch (error) {
      console.error('Failed to delete user:', error)
      alert(error instanceof Error ? error.message : 'Failed to delete user')
    }
  }

  const handleUpdateRole = async (newRole: 'admin' | 'user') => {
    if (!selectedUser) return
    try {
      await updateUser.mutateAsync({
        id: selectedUser.id,
        data: { role: newRole },
      })
      setEditDialogOpen(false)
      setSelectedUser(null)
    } catch (error) {
      console.error('Failed to update user:', error)
      alert(error instanceof Error ? error.message : 'Failed to update user')
    }
  }

  const handleUpdateStatus = async (newStatus: 'active' | 'suspended' | 'deleted') => {
    if (!selectedUser) return
    try {
      await updateUser.mutateAsync({
        id: selectedUser.id,
        data: { status: newStatus },
      })
      setEditDialogOpen(false)
      setSelectedUser(null)
    } catch (error) {
      console.error('Failed to update user:', error)
      alert(error instanceof Error ? error.message : 'Failed to update user')
    }
  }

  const totalPages = data ? Math.ceil(data.total / limit) : 1

  const columns = useMemo(
    () => [
      columnHelper.accessor('name', {
        header: 'Name',
        cell: (info) => <span className="font-medium">{info.getValue()}</span>,
      }),
      columnHelper.accessor('email', {
        header: 'Email',
      }),
      columnHelper.accessor('role', {
        header: 'Role',
        cell: (info) => <RoleBadge role={info.getValue()} />,
      }),
      columnHelper.accessor('status', {
        header: 'Status',
        cell: (info) => <StatusBadge status={info.getValue()} />,
      }),
      columnHelper.accessor('created_at', {
        header: 'Created',
        cell: (info) => (
          <span className="text-sm text-muted-foreground">
            {new Date(info.getValue()).toLocaleDateString()}
          </span>
        ),
      }),
      columnHelper.display({
        id: 'actions',
        header: () => <span className="flex justify-end">Actions</span>,
        cell: (info) => {
          const user = info.row.original
          return (
            <div className="flex justify-end">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="sm">
                    <MoreVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => handleEditUser(user)}>
                    <Shield className="mr-2 h-4 w-4" />
                    Edit User
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onClick={() => handleDeleteUser(user)}
                  >
                    <Trash2 className="mr-2 h-4 w-4" />
                    Delete User
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          )
        },
      }),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  const table = useReactTable({
    data: data?.users ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    pageCount: totalPages,
  })

  return (
    <div className="min-h-screen bg-background py-8">
      <div className="container mx-auto max-w-[1400px] px-6">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-6 flex items-center gap-2">
            <UsersIcon className="h-6 w-6 text-muted-foreground" />
            <h1 className="text-2xl font-medium tracking-tight">User Management</h1>
          </div>

          {/* Filters */}
          <div className="flex flex-col gap-4 rounded-xl border border-border bg-card p-4 sm:flex-row">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search by email or name..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>

            <Select value={roleFilter} onValueChange={setRoleFilter}>
              <SelectTrigger className="w-full sm:w-[140px]">
                <SelectValue placeholder="Role" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Roles</SelectItem>
                <SelectItem value="admin">Admin</SelectItem>
                <SelectItem value="user">User</SelectItem>
              </SelectContent>
            </Select>

            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-full sm:w-[140px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="suspended">Suspended</SelectItem>
                <SelectItem value="deleted">Deleted</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Table */}
        <div className="rounded-xl border border-border bg-card">
          {isLoading ? (
            <div className="flex items-center justify-center py-20">
              <div className="text-lg">Loading...</div>
            </div>
          ) : !data || data.users.length === 0 ? (
            <div className="flex items-center justify-center py-20 text-muted-foreground">
              No users found
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  {table.getHeaderGroups().map((headerGroup) => (
                    <TableRow key={headerGroup.id}>
                      {headerGroup.headers.map((header) => (
                        <TableHead key={header.id}>
                          {flexRender(header.column.columnDef.header, header.getContext())}
                        </TableHead>
                      ))}
                    </TableRow>
                  ))}
                </TableHeader>
                <TableBody>
                  {table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id}>
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>

              {/* Pagination */}
              <div className="flex items-center justify-between border-t border-border px-6 py-4">
                <div className="text-sm text-muted-foreground">
                  Showing {(page - 1) * limit + 1} to {Math.min(page * limit, data.total)} of{' '}
                  {data.total} users
                </div>
                <div className="flex gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage(page - 1)}
                    disabled={page === 1}
                  >
                    Previous
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPage(page + 1)}
                    disabled={page >= totalPages}
                  >
                    Next
                  </Button>
                </div>
              </div>
            </>
          )}
        </div>
      </div>

      {/* Edit User Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={setEditDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit User</DialogTitle>
            <DialogDescription>
              Update user role and status for {selectedUser?.email}
            </DialogDescription>
          </DialogHeader>
          {selectedUser && (
            <div className="space-y-6 py-4">
              {/* User Info */}
              <div className="space-y-2 rounded-lg border border-border bg-muted/50 p-4">
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Email:</span>
                  <span className="text-sm font-medium">{selectedUser.email}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Name:</span>
                  <span className="text-sm font-medium">{selectedUser.name}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">Created:</span>
                  <span className="text-sm font-medium">
                    {new Date(selectedUser.created_at).toLocaleDateString()}
                  </span>
                </div>
              </div>

              {/* Role Selection */}
              <div className="space-y-3">
                <label className="text-sm font-medium">Role</label>
                <div className="flex gap-2">
                  <Button
                    variant={selectedUser.role === 'user' ? 'default' : 'outline'}
                    className="flex-1"
                    onClick={() => handleUpdateRole('user')}
                    disabled={updateUser.isPending}
                  >
                    User
                  </Button>
                  <Button
                    variant={selectedUser.role === 'admin' ? 'default' : 'outline'}
                    className="flex-1"
                    onClick={() => handleUpdateRole('admin')}
                    disabled={updateUser.isPending}
                  >
                    Admin
                  </Button>
                </div>
              </div>

              {/* Status Selection */}
              <div className="space-y-3">
                <label className="text-sm font-medium">Status</label>
                <div className="flex flex-col gap-2">
                  <Button
                    variant={selectedUser.status === 'active' ? 'default' : 'outline'}
                    onClick={() => handleUpdateStatus('active')}
                    disabled={updateUser.isPending}
                    className="justify-start"
                  >
                    <CheckCircle className="mr-2 h-4 w-4" />
                    Active
                  </Button>
                  <Button
                    variant={selectedUser.status === 'suspended' ? 'default' : 'outline'}
                    onClick={() => handleUpdateStatus('suspended')}
                    disabled={updateUser.isPending}
                    className="justify-start"
                  >
                    <Ban className="mr-2 h-4 w-4" />
                    Suspended
                  </Button>
                </div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditDialogOpen(false)}>
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete User</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete <strong>{selectedUser?.email}</strong>? This will set
              their status to &apos;deleted&apos; and prevent them from logging in. This action can
              be reversed by changing the status back to &apos;active&apos;.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={confirmDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Delete User
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// Badge Components
function RoleBadge({ role }: { role: string }) {
  const isAdmin = role === 'admin'
  return (
    <Badge variant={isAdmin ? 'default' : 'secondary'} className="capitalize">
      {role}
    </Badge>
  )
}

function StatusBadge({ status }: { status: string }) {
  const variant =
    status === 'active' ? 'default' :
    status === 'suspended' ? 'secondary' :
    'destructive'

  return (
    <Badge variant={variant} className="capitalize">
      {status}
    </Badge>
  )
}
