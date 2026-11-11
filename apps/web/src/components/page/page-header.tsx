import type { ReactNode } from 'react'
import { Flex, Typography } from 'antd'

interface PageHeaderProps {
  title: string
  /** 一句话说明这页在做什么，放在标题下面；没有就别传。 */
  description?: string
  /** 右侧操作区（新建、导出之类）。 */
  extra?: ReactNode
}

/**
 * 页面标题栏。
 *
 * 抽出来是为了让每个页面的「标题 — 说明 — 操作」位置一致：
 * 各页自己写 h2 + 按钮，最后总会差几个像素，改一次全局样式要翻遍所有页面。
 */
export function PageHeader({ title, description, extra }: PageHeaderProps) {
  return (
    <Flex align="flex-start" justify="space-between" style={{ marginBottom: 16 }} gap={16}>
      <div>
        <Typography.Title level={4} style={{ margin: 0 }}>
          {title}
        </Typography.Title>
        {description ? (
          <Typography.Text type="secondary">{description}</Typography.Text>
        ) : null}
      </div>
      {extra}
    </Flex>
  )
}
