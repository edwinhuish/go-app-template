import { Spin } from 'antd'

/** 整页加载态：用于「还不知道用户登不登录」这类不能先渲染局部的情况。 */
export function FullPageLoading() {
  return (
    <div
      style={{
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Spin size="large" />
    </div>
  )
}
