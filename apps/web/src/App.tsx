import { BrowserRouter } from 'react-router'
import { App as AntApp, ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'

import { AuthProvider } from '@/providers/auth'
import { AppRoutes } from '@/router'
import { themeConfig } from '@/theme'

/**
 * 应用根。
 *
 * 四层包裹，顺序有讲究：
 *   BrowserRouter —— 路由上下文，登录页与受保护段都在它下面
 *   ConfigProvider —— 全局主题与中文语言包
 *   AntApp —— 提供 message / modal / notification 的上下文实例
 *             （直接 import 静态方法会在控制台报「context 不匹配」的警告）
 *   AuthProvider —— 登录态，路由守卫要读它，所以必须在 Routes 外层
 */
export default function App() {
  return (
    <BrowserRouter>
      <ConfigProvider locale={zhCN} theme={themeConfig}>
        <AntApp>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </AntApp>
      </ConfigProvider>
    </BrowserRouter>
  )
}
