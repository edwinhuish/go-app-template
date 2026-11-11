// 前端类型定义集中在这里。
//
// 与后端 dto 一一对应：字段名保持一致，改接口时两边一起改，
// 不要在前端另起一套「更顺手」的名字——那会让每次联调都要查一遍对应关系。

/** 后端统一分页响应里的 data 部分。 */
export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

/** 示例实体。 */
export interface NoteItem {
  id: number
  title: string
  content: string
  owner_id: number
  created_at: string
  updated_at: string
}

/** 当前登录账号。 */
export interface UserInfo {
  id: number
  username: string
  nickname: string
  role: string
}

/** 登录成功的响应体。 */
export interface LoginResult {
  token: string
  expires_at: number
  user: UserInfo
}
