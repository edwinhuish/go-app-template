import type { ReactNode } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router'

import { AppLayout } from '@/components/layout/AppLayout'
import { FullPageLoading } from '@/components/ui/loading'
import { LoginPage } from '@/pages/login'
import { NotFoundPage } from '@/pages/not-found'
import { NotesPage } from '@/pages/notes'
import { useAuth } from '@/providers/auth'

/**
 * 受保护路由：未登录一律送回登录页，并把当前地址记进 state，
 * 登录成功后可以直接回到原处。
 *
 * loading 期间必须只渲染加载态：否则「有令牌但还没校验完」的那一瞬间，
 * user 是 null，用户会被弹回登录页——表现为登录后刷新总被踢出来。
 */
function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()
  const location = useLocation()

  if (loading) return <FullPageLoading />
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <>{children}</>
}

/** 路由表。加页面时在这里挂一条，并在 config/nav.ts 里加菜单项。 */
export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route
        path="/"
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/notes" replace />} />
        <Route path="notes" element={<NotesPage />} />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
