import { createFileRoute } from '@tanstack/react-router'
import { useState, useMemo } from 'react'
import {
  useReactTable,
  getCoreRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
  type RowSelectionState,
} from '@tanstack/react-table'
import { Users as UsersIcon, Search, MoreVertical, Trash2, Ban, CheckCircle, ArrowUpDown, ArrowUp, ArrowDown, Pencil } from 'lucide-react'
import { useUsers, useUpdateUser, useDeleteUser, useBulkUpdateUsers } from '../lib/api/hooks'
import { requireAdmin } from '../lib/auth'
import { Spinner } from '#/components/ui/spinner'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Badge } from '#/components/ui/badge'
import { Checkbox } from '#/components/ui/checkbox'
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
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
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
  const [sorting, setSorting] = useState<SortingState>([])
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [userToDelete, setUserToDelete] = useState<UserDetail | null>(null)

  const limit = 20

  const sortBy = sorting[0]?.id
  const sortDir = sorting[0] ? (sorting[0].desc ? 'desc' : 'asc') : undefined

  const { data, isLoading } = useUsers({
    page,
    limit,
    search: search || undefined,
    role: roleFilter !== 'all' ? roleFilter : undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
    sort_by: sortBy,
    sort_dir: sortDir,
  })

  const updateUser = useUpdateUser()
  const deleteUser = useDeleteUser()
  const bulkUpdate = useBulkUpdateUsers()

  const handleDeleteUser = (user: UserDetail) => {
    setUserToDelete(user)
    setDeleteDialogOpen(true)
  }

  const confirmDelete = async () => {
    if (!userToDelete) return
    try {
      await deleteUser.mutateAsync(userToDelete.id)
      setDeleteDialogOpen(false)
      setUserToDelete(null)
    } catch (error) {
      console.error('Failed to delete user:', error)
      alert(error instanceof Error ? error.message : 'Failed to delete user')
    }
  }

  const handleUpdateRole = async (userId: string, newRole: 'admin' | 'user') => {
    try {
      await updateUser.mutateAsync({ id: userId, data: { role: newRole } })
    } catch (error) {
      console.error('Failed to update user:', error)
      alert(error instanceof Error ? error.message : 'Failed to update user')
    }
  }

  const handleUpdateStatus = async (userId: string, newStatus: 'active' | 'suspended' | 'deleted') => {
    try {
      await updateUser.mutateAsync({ id: userId, data: { status: newStatus } })
    } catch (error) {
      console.error('Failed to update user:', error)
      alert(error instanceof Error ? error.message : 'Failed to update user')
    }
  }

  const handleBulkAction = async (action: 'suspend' | 'activate' | 'delete') => {
    const selectedIds = Object.keys(rowSelection).map(
      (idx) => data!.users[parseInt(idx)].id,
    )
    const statusMap = { suspend: 'suspended', activate: 'active', delete: 'deleted' } as const
    try {
      await bulkUpdate.mutateAsync({ ids: selectedIds, status: statusMap[action] })
      setRowSelection({})
    } catch (error) {
      console.error('Bulk action failed:', error)
      alert(error instanceof Error ? error.message : 'Bulk action failed')
    }
  }

  const totalPages = data ? Math.ceil(data.total / limit) : 1
  const selectedCount = Object.keys(rowSelection).length

  const columns = useMemo(
    () => [
      columnHelper.display({
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllRowsSelected()}
            onCheckedChange={(checked) => table.toggleAllRowsSelected(!!checked)}
            aria-label="Select all"
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(checked) => row.toggleSelected(!!checked)}
            aria-label="Select row"
          />
        ),
      }),
      columnHelper.accessor('name', {
        header: ({ column }) => <SortHeader label="Name" column={column} />,
        cell: (info) => <span className="font-medium">{info.getValue()}</span>,
      }),
      columnHelper.accessor('email', {
        header: ({ column }) => <SortHeader label="Email" column={column} />,
      }),
      columnHelper.accessor('role', {
        header: ({ column }) => <SortHeader label="Role" column={column} />,
        cell: (info) => (
          <InlineRoleSelect
            userId={info.row.original.id}
            role={info.getValue()}
            onUpdate={handleUpdateRole}
            isPending={updateUser.isPending}
          />
        ),
      }),
      columnHelper.accessor('status', {
        header: ({ column }) => <SortHeader label="Status" column={column} />,
        cell: (info) => (
          <InlineStatusSelect
            userId={info.row.original.id}
            status={info.getValue()}
            onUpdate={handleUpdateStatus}
            isPending={updateUser.isPending}
          />
        ),
      }),
      columnHelper.accessor('created_at', {
        header: ({ column }) => <SortHeader label="Created" column={column} />,
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
    state: { sorting, rowSelection },
    onSortingChange: (updater) => {
      setSorting(updater)
      setPage(1)
    },
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    pageCount: totalPages,
  })

  return (
    <div className="h-full overflow-auto bg-background py-8">
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

        {/* Bulk Action Bar */}
        {selectedCount > 0 && (
          <div className="mb-4 flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
            <span className="text-sm text-muted-foreground">
              {selectedCount} user{selectedCount !== 1 ? 's' : ''} selected
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleBulkAction('activate')}
                disabled={bulkUpdate.isPending}
              >
                <CheckCircle className="mr-2 h-4 w-4" />
                Activate
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => handleBulkAction('suspend')}
                disabled={bulkUpdate.isPending}
              >
                <Ban className="mr-2 h-4 w-4" />
                Suspend
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => handleBulkAction('delete')}
                disabled={bulkUpdate.isPending}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </Button>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto"
              onClick={() => setRowSelection({})}
            >
              Clear
            </Button>
          </div>
        )}

        {/* Table */}
        <div className="rounded-xl border border-border bg-card">
          {isLoading ? (
            <div className="flex items-center justify-center py-20">
              <Spinner className="size-6 text-muted-foreground" />
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

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete User</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete <strong>{userToDelete?.email}</strong>? This will set
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

// SortHeader renders a clickable column header with sort indicator
function SortHeader({ label, column }: { label: string; column: { getIsSorted: () => false | 'asc' | 'desc'; toggleSorting: (desc?: boolean) => void } }) {
  const sorted = column.getIsSorted()
  return (
    <button
      className="flex items-center gap-1 hover:text-foreground"
      onClick={() => column.toggleSorting(sorted === 'asc')}
    >
      {label}
      {sorted === 'asc' ? (
        <ArrowUp className="h-3 w-3" />
      ) : sorted === 'desc' ? (
        <ArrowDown className="h-3 w-3" />
      ) : (
        <ArrowUpDown className="h-3 w-3 opacity-40" />
      )}
    </button>
  )
}

// Inline Role Select Component
function InlineRoleSelect({
  userId,
  role,
  onUpdate,
  isPending,
}: {
  userId: string
  role: 'admin' | 'user'
  onUpdate: (userId: string, role: 'admin' | 'user') => Promise<void>
  isPending: boolean
}) {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <div className="relative inline-flex">
      <button
        onClick={() => setIsOpen(true)}
        disabled={isPending}
        className="group inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium transition-colors hover:bg-muted/50 disabled:opacity-50"
      >
        <Badge variant={role === 'admin' ? 'default' : 'secondary'} className="capitalize group-hover:opacity-70">
          {role}
        </Badge>
        <Pencil className="h-3 w-3 opacity-0 transition-opacity group-hover:opacity-100" />
      </button>
      {isOpen && (
        <Select
          value={role}
          onValueChange={(value) => {
            onUpdate(userId, value as 'admin' | 'user')
            setIsOpen(false)
          }}
          open={true}
          onOpenChange={(open) => {
            if (!open) setIsOpen(false)
          }}
        >
          <SelectTrigger className="absolute inset-0 h-full w-full opacity-0">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="user">User</SelectItem>
            <SelectItem value="admin">Admin</SelectItem>
          </SelectContent>
        </Select>
      )}
    </div>
  )
}

// Inline Status Select Component
function InlineStatusSelect({
  userId,
  status,
  onUpdate,
  isPending,
}: {
  userId: string
  status: 'active' | 'suspended' | 'deleted'
  onUpdate: (userId: string, status: 'active' | 'suspended' | 'deleted') => Promise<void>
  isPending: boolean
}) {
  const [isOpen, setIsOpen] = useState(false)

  const getVariant = () => {
    if (status === 'active') return 'default'
    if (status === 'suspended') return 'secondary'
    return 'destructive'
  }

  const getStatusIcon = () => {
    if (status === 'active') return <CheckCircle className="h-3 w-3" />
    if (status === 'suspended') return <Ban className="h-3 w-3" />
    return null
  }

  return (
    <div className="relative inline-flex">
      <button
        onClick={() => setIsOpen(true)}
        disabled={isPending}
        className="group inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium transition-colors hover:bg-muted/50 disabled:opacity-50"
      >
        <Badge variant={getVariant()} className="capitalize group-hover:opacity-70">
          <span className="flex items-center gap-1">
            {getStatusIcon()}
            {status}
          </span>
        </Badge>
        <Pencil className="h-3 w-3 opacity-0 transition-opacity group-hover:opacity-100" />
      </button>
      {isOpen && (
        <Select
          value={status}
          onValueChange={(value) => {
            onUpdate(userId, value as 'active' | 'suspended' | 'deleted')
            setIsOpen(false)
          }}
          open={true}
          onOpenChange={(open) => {
            if (!open) setIsOpen(false)
          }}
        >
          <SelectTrigger className="absolute inset-0 h-full w-full opacity-0">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">
              <span className="flex items-center gap-2">
                <CheckCircle className="h-3 w-3" />
                Active
              </span>
            </SelectItem>
            <SelectItem value="suspended">
              <span className="flex items-center gap-2">
                <Ban className="h-3 w-3" />
                Suspended
              </span>
            </SelectItem>
            <SelectItem value="deleted">Deleted</SelectItem>
          </SelectContent>
        </Select>
      )}
    </div>
  )
}
