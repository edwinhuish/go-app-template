// 登录态：谁在登录、怎么登入登出。
//
// 用 Context 而不是第三方状态库：这份状态只有一份、改动点只有三处
// （初始化、登录、登出），引入一个库反而多一层心智负担。
// 将来要接权限、多标签页同步时，再在这里扩展。

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import * as authApi from '@/api/auth'
import { ApiError, getToken, setToken } from '@/api/client'
import type { UserInfo } from '@/types'

interface AuthContextValue {
  user: UserInfo | null
  /** 首次校验令牌期间为 true，用来避免「已登录却被弹回登录页」的闪烁。 */
  loading: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<UserInfo | null>(null)
  const [loading, setLoading] = useState(() => getToken() !== null)

  // 带着本地令牌进页面时，先向服务端确认它还有效：
  // 令牌可能已过期、也已因登出被删除，光看 localStorage 里有没有值不够。
  useEffect(() => {
    if (getToken() === null) return

    let cancelled = false
    authApi
      .profile()
      .then((info) => {
        if (!cancelled) setUser(info)
      })
      .catch((err: unknown) => {
        // 401 已在 client 里清掉令牌；其它错误（网络抖动）保留令牌，
        // 下次刷新还能恢复，不至于把用户直接踢下线。
        if (err instanceof ApiError && err.code !== 40101) {
          console.warn('校验登录态失败', err.message)
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [])

  const login = useCallback(async (username: string, password: string) => {
    const res = await authApi.login(username, password)
    setToken(res.token)
    setUser(res.user)
  }, [])

  const logout = useCallback(async () => {
    try {
      await authApi.logout()
    } finally {
      // 服务端删除会话失败也要清本地状态：否则界面停在「已登录」，
      // 而每个请求都 401，用户只能手动清缓存。
      setToken(null)
      setUser(null)
    }
  }, [])

  const value = useMemo(
    () => ({ user, loading, login, logout }),
    [user, loading, login, logout],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

/** 取登录态。必须在 AuthProvider 内部使用。 */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth 必须在 AuthProvider 内使用')
  }
  return ctx
}
