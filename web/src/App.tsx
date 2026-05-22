import { Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './hooks/useAuth'
import AppLayout from './layouts/AppLayout'
import LoginPage from './pages/LoginPage'
import RegisterPage from './pages/RegisterPage'
import ChatPage from './pages/ChatPage'
import KnowledgePage from './pages/KnowledgePage'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth()
  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-gray-50 dark:bg-zinc-950">
        <span className="text-gray-400 dark:text-zinc-600 text-sm">Loading...</span>
      </div>
    )
  }
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/app" element={<ProtectedRoute><AppLayout /></ProtectedRoute>}>
          <Route index element={<Navigate to="chat" replace />} />
          <Route path="chat" element={<ChatPage />} />
          <Route path="knowledge" element={<KnowledgePage />} />
          <Route path="agents" element={<Navigate to="chat" replace />} />
          <Route path="skills" element={<Navigate to="chat" replace />} />
        </Route>
        <Route path="*" element={<Navigate to="/app/chat" replace />} />
      </Routes>
    </AuthProvider>
  )
}
