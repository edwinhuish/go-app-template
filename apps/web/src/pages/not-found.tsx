import { Button, Result } from 'antd'
import { useNavigate } from 'react-router'

/** 兜底页：地址写错时给一个明确的出口，而不是空白。 */
export function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <Result
      status="404"
      title="404"
      subTitle="页面不存在"
      extra={
        <Button type="primary" onClick={() => navigate('/notes', { replace: true })}>
          回到首页
        </Button>
      }
    />
  )
}
