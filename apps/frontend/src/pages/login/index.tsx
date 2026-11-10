import { useState } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { LockOutlined, UserOutlined } from '@ant-design/icons'
import { App as AntApp, Button, Card, Form, Input, Typography } from 'antd'

import { useAuth } from '@/providers/auth'

interface LoginFormValues {
  username: string
  password: string
}

/**
 * 登录页。
 *
 * 不套 AppLayout：未登录时不该看到导航，否则点进去每一条都是 401。
 * 路由层已经把它放在受保护段之外，这里只管表单。
 */
export function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const { message } = AntApp.useApp()
  const [submitting, setSubmitting] = useState(false)

  const onFinish = async (values: LoginFormValues) => {
    setSubmitting(true)
    try {
      await login(values.username, values.password)

      // 被弹回登录页时记住原地址，登进来直接回原处，而不是一律回首页。
      const from = (location.state as { from?: string } | null)?.from
      navigate(from && from !== '/login' ? from : '/notes', { replace: true })
    } catch (err) {
      message.error(err instanceof Error ? err.message : '登录失败')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#f5f5f5',
      }}
    >
      <Card style={{ width: 360 }}>
        <Typography.Title level={4} style={{ marginTop: 0, textAlign: 'center' }}>
          登录
        </Typography.Title>

        <Form<LoginFormValues> layout="vertical" onFinish={onFinish} requiredMark={false}>
          <Form.Item
            name="username"
            label="账号"
            rules={[{ required: true, message: '请输入账号' }]}
          >
            <Input prefix={<UserOutlined />} placeholder="账号" autoComplete="username" />
          </Form.Item>
          <Form.Item
            name="password"
            label="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
              autoComplete="current-password"
            />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={submitting}>
            登录
          </Button>
        </Form>

        <Typography.Paragraph type="secondary" style={{ margin: '16px 0 0', fontSize: 12 }}>
          开发环境默认账号 admin / admin123，见 apps/backend/configs/config.yaml。
        </Typography.Paragraph>
      </Card>
    </div>
  )
}
