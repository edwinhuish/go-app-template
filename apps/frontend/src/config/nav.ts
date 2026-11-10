// 侧边栏导航。菜单与路由共用这份清单：
// 加一个页面时改这里 + router/index.tsx 两处，标题只维护一份。

import { createElement } from 'react'
import type { ReactNode } from 'react'
import { FileTextOutlined } from '@ant-design/icons'

export interface NavItem {
  /** 路由路径，同时用作菜单 key。 */
  path: string
  label: string
  icon: ReactNode
}

export const navItems: NavItem[] = [
  {
    path: '/notes',
    label: '示例模块',
    // createElement 而不是 JSX：本文件是 .ts（不含 JSX 语法），
    // 而 antd Menu 的 items 需要的是元素而不是组件类型。
    icon: createElement(FileTextOutlined),
  },
]
