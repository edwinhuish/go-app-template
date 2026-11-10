import { http } from '@/api/client'
import type { LoginResult, UserInfo } from '@/types'

/** 登录，返回令牌与账号信息。 */
export function login(username: string, password: string) {
  return http.post<LoginResult>('/auth/login', { username, password })
}

/** 取当前登录身份。令牌失效时会抛 ApiError(401)，由调用方清登录态。 */
export function profile() {
  return http.get<UserInfo>('/auth/profile')
}

/** 登出，服务端删除会话。 */
export function logout() {
  return http.post<{ logged_out: boolean }>('/auth/logout')
}
