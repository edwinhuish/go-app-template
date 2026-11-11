import { useMemo } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { LogoutOutlined } from '@ant-design/icons'
import { Avatar, Button, Layout, Menu, Space, Typography } from 'antd'

import { navItems } from '@/config/nav'
import { useAuth } from '@/providers/auth'

const { Header, Sider, Content } = Layout

/**
 * 应用外壳：左侧导航 + 顶部账号区 + 内容区。
 *
 * 路由内容通过 <Outlet /> 渲染，外壳本身不关心有哪些页面——
 * 加页面只改 config/nav.ts 与 router/index.tsx。
 */
export function AppLayout() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()

  // Menu 的选中项按路径前缀匹配：/notes/123 时仍然高亮「示例模块」。
  const selectedKey = useMemo(() => {
    const hit = navItems
      .filter((item) => location.pathname.startsWith(item.path))
      .sort((a, b) => b.path.length - a.path.length)[0]
    return hit?.path ?? location.pathname
  }, [location.pathname])

  return (
    <Layout style={{ minHeight: '100%' }}>
      <Sider theme="light" width={220} style={{ borderRight: '1px solid #f0f0f0' }}>
        <div style={{ height: 56, display: 'flex', alignItems: 'center', padding: '0 20px' }}>
          <Typography.Text strong style={{ fontSize: 16 }}>
            App
          </Typography.Text>
        </div>
        <Menu
          mode="inline"
          selectedKeys={[selectedKey]}
          items={navItems.map((item) => ({
            key: item.path,
            icon: item.icon,
            label: item.label,
          }))}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>

      <Layout>
        <Header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'flex-end',
            padding: '0 24px',
            borderBottom: '1px solid #f0f0f0',
          }}
        >
          <Space size={12}>
            <Avatar size="small">{user?.nickname?.[0] ?? user?.username?.[0] ?? '?'}</Avatar>
            <Typography.Text>{user?.nickname || user?.username}</Typography.Text>
            <Button
              type="text"
              icon={<LogoutOutlined />}
              onClick={() => {
                // 登出后回登录页；放在 then 里而不是依赖 useEffect 监听 user，
                // 是因为这里明确知道「这次跳转的原因」。
                void logout().then(() => navigate('/login', { replace: true }))
              }}
            >
              退出
            </Button>
          </Space>
        </Header>

        <Content style={{ padding: 24, background: '#f5f5f5' }}>
          <div style={{ background: '#fff', padding: 24, borderRadius: 8 }}>
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  )
}
