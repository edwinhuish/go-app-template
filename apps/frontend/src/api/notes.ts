import { http } from '@/api/client'
import type { NoteItem, PageResult } from '@/types'
import { toQuery } from '@/utils/format'

/** 列表查询参数。 */
export interface NoteListParams {
  page?: number
  page_size?: number
  keyword?: string
}

/** 新建 / 更新的入参。 */
export interface NoteInput {
  title: string
  content?: string
  /** 更新时带上读到的 updated_at，服务端据此做乐观锁。 */
  updated_at?: string
}

export function listNotes(params: NoteListParams) {
  return http.get<PageResult<NoteItem>>(`/notes${toQuery({ ...params })}`)
}

export function createNote(input: NoteInput) {
  return http.post<NoteItem>('/notes', input)
}

export function updateNote(id: number, input: NoteInput) {
  return http.put<NoteItem>(`/notes/${id}`, input)
}

export function deleteNote(id: number) {
  return http.delete<{ deleted: boolean }>(`/notes/${id}`)
}
