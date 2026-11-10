import { useState } from 'react'
import { PlusOutlined } from '@ant-design/icons'
import {
  Alert,
  App as AntApp,
  Button,
  Form,
  Input,
  Modal,
  Popconfirm,
  Space,
  Table,
  Typography,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'

import { createNote, deleteNote, listNotes, updateNote } from '@/api/notes'
import { PageHeader } from '@/components/page'
import { DEFAULT_PAGE_SIZE } from '@/constants'
import { useList } from '@/hooks/use-list'
import type { NoteItem } from '@/types'
import { formatDateTime } from '@/utils/format'

interface NoteFormValues {
  title: string
  content?: string
}

/**
 * 示例列表页。
 *
 * 这一页演示的是「一个列表页最少要有哪几件事」：
 * 查询条件、分页、新建/编辑、删除、错误提示。
 * 接真实业务时按同样的骨架替换字段即可。
 */
export function NotesPage() {
  const { message } = AntApp.useApp()

  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')

  // 查询参数放进 deps：page 或 keyword 一变，hook 自动重新取数。
  const { data, loading, error, reload } = useList(
    () => listNotes({ page, page_size: DEFAULT_PAGE_SIZE, keyword }),
    [page, keyword],
  )

  const [form] = Form.useForm<NoteFormValues>()
  const [open, setOpen] = useState(false)
  const [editing, setEditing] = useState<NoteItem | null>(null)
  const [saving, setSaving] = useState(false)

  const openCreate = () => {
    setEditing(null)
    form.resetFields()
    setOpen(true)
  }

  const openEdit = (record: NoteItem) => {
    setEditing(record)
    form.setFieldsValue({ title: record.title, content: record.content })
    setOpen(true)
  }

  const submit = async () => {
    const values = await form.validateFields()
    setSaving(true)
    try {
      if (editing) {
        // 带上 updated_at：服务端据此做乐观锁，别人先改过会返回 409。
        await updateNote(editing.id, { ...values, updated_at: editing.updated_at })
      } else {
        await createNote(values)
      }
      message.success(editing ? '已保存' : '已创建')
      setOpen(false)
      reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const remove = async (record: NoteItem) => {
    try {
      await deleteNote(record.id)
      message.success('已删除')
      reload()
    } catch (err) {
      message.error(err instanceof Error ? err.message : '删除失败')
    }
  }

  const columns: ColumnsType<NoteItem> = [
    { title: '标题', dataIndex: 'title', width: 240 },
    {
      title: '内容',
      dataIndex: 'content',
      render: (value: string) => (
        <Typography.Text className="table-cell-wrap">{value || '-'}</Typography.Text>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      width: 160,
      render: (value: string) => formatDateTime(value),
    },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space size={4}>
          <Button type="link" size="small" onClick={() => openEdit(record)}>
            编辑
          </Button>
          <Popconfirm
            title="删除这条记录？"
            description="删除后不可在界面上恢复。"
            okText="删除"
            cancelText="取消"
            okButtonProps={{ danger: true }}
            onConfirm={() => void remove(record)}
          >
            <Button type="link" size="small" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="示例模块"
        description="演示 model → repository → service → handler 的完整链路，接业务时替换掉。"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            新建
          </Button>
        }
      />

      <Space style={{ marginBottom: 16 }}>
        <Input.Search
          allowClear
          placeholder="按标题搜索"
          style={{ width: 240 }}
          onSearch={(value) => {
            setKeyword(value.trim())
            // 换条件必须回到第一页：停在第 3 页而新条件只有 1 页时，
            // 列表会是空的，看起来像「没搜到」。
            setPage(1)
          }}
        />
      </Space>

      {error ? (
        <Alert type="error" showIcon message={error} style={{ marginBottom: 16 }} />
      ) : null}

      <Table<NoteItem>
        rowKey="id"
        size="middle"
        loading={loading}
        columns={columns}
        dataSource={data?.list ?? []}
        pagination={{
          current: page,
          pageSize: DEFAULT_PAGE_SIZE,
          total: data?.total ?? 0,
          showSizeChanger: false,
          onChange: setPage,
        }}
      />

      <Modal
        title={editing ? '编辑' : '新建'}
        open={open}
        confirmLoading={saving}
        onOk={() => void submit()}
        onCancel={() => setOpen(false)}
        destroyOnHidden
      >
        <Form<NoteFormValues> form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="title"
            label="标题"
            rules={[{ required: true, message: '请输入标题' }, { max: 128, message: '最长 128 字' }]}
          >
            <Input placeholder="标题" />
          </Form.Item>
          <Form.Item name="content" label="内容">
            <Input.TextArea rows={4} placeholder="内容" />
          </Form.Item>
        </Form>
      </Modal>
    </>
  )
}
