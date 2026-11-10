import type { ThemeConfig } from 'antd'

/**
 * 全局主题。
 *
 * 只放「整站一致」的东西：主色、圆角、组件默认尺寸。
 * 单个页面里的样式用组件自身的 props 表达，不要往这里堆例外——
 * 例外一多，改主题就变成逐个排查哪条覆盖生效。
 */
export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: '#1677ff',
    borderRadius: 6,
  },
  components: {
    Layout: {
      // 侧边栏用浅色：深色侧栏与白色表格放一起时，视觉重心会压在导航上。
      siderBg: '#ffffff',
      headerBg: '#ffffff',
    },
  },
}
