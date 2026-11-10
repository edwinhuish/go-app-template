// 统一的 HTTP 客户端：注入令牌、拆包 {code,message,data}、把业务错误变成异常。
//
// 所有页面都从这里出去，所以「带令牌」「判错误码」「401 清登录态」只写一遍。
// 换成 axios / react-query 时，也替换这个文件即可，页面不必改。

import { API_BASE, TOKEN_KEY } from '@/constants'

/** 业务错误：带后端的 code，页面可以据此做分支（比如 409 提示刷新重试）。 */
export class ApiError extends Error {
  code: number

  constructor(message: string, code: number) {
    super(message)
    this.name = 'ApiError'
    this.code = code
  }
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null) {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

interface ApiBody<T> {
  code: number
  message: string
  data: T
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (!(init?.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json')
  }

  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  let res: Response
  try {
    res = await fetch(`${API_BASE}${path}`, { ...init, headers })
  } catch {
    // fetch 只在网络层失败时 reject，这里统一成一句人话：
    // 后端没起、代理配错、断网都会走到这。
    throw new ApiError('网络异常，请检查后端服务是否已启动', -1)
  }

  const body = (await res.json().catch(() => null)) as ApiBody<T> | null

  if (!res.ok || !body || body.code !== 0) {
    // 401 时清掉本地令牌：留着它，页面会一直以为自己是登录态。
    if (res.status === 401) {
      setToken(null)
    }
    throw new ApiError(body?.message || `请求失败 (${res.status})`, body?.code ?? res.status)
  }

  return body.data
}

export const http = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: JSON.stringify(body ?? {}) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: JSON.stringify(body ?? {}) }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
