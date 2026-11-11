import { useCallback, useEffect, useState } from 'react'

import type { PageResult } from '@/types'

interface ListState<T> {
  data: PageResult<T> | null
  loading: boolean
  error: string | null
}

/**
 * 通用列表加载 hook：负责 loading / error / 重新加载三件事。
 *
 * deps 由调用方给出——这是一个通用 hook，静态分析无法知道该依赖什么；
 * 漏写依赖的表现是「改了筛选条件但列表没刷新」，加依赖即可。
 */
export function useList<T>(load: () => Promise<PageResult<T>>, deps: unknown[]) {
  const [state, setState] = useState<ListState<T>>({ data: null, loading: true, error: null })
  const [version, setVersion] = useState(0)

  useEffect(() => {
    // cancelled 用来丢弃过期响应：快速切换筛选条件时，
    // 先发出的请求可能后返回，不做标记会出现「列表内容与筛选条件对不上」。
    let cancelled = false
    setState((prev) => ({ ...prev, loading: true, error: null }))

    load()
      .then((data) => {
        if (!cancelled) setState({ data, loading: false, error: null })
      })
      .catch((err: unknown) => {
        if (cancelled) return
        const message = err instanceof Error ? err.message : '加载失败'
        setState({ data: null, loading: false, error: message })
      })

    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version, ...deps])

  const reload = useCallback(() => setVersion((v) => v + 1), [])

  return { ...state, reload }
}
