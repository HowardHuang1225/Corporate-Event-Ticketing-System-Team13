import { createContext, useContext, useState, useEffect, type ReactNode } from 'react'
import api from '../api/client'
import { useQueryClient } from '@tanstack/react-query'

export interface User {
  id: string
  employee_id: string
  name: string
  email: string
  department: string
  region: string
  role: 'employee' | 'event_manager' | 'hr'
}

interface AuthContextType {
  user: User | null
  login: (employeeId: string, password: string) => Promise<void>
  logout: () => void
  isLoading: boolean
}

const AuthContext = createContext<AuthContextType | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const queryClient = useQueryClient()

  useEffect(() => {
    const token = localStorage.getItem('token')
    if (!token) { setIsLoading(false); return }
    api.get('/auth/me')
      .then(res => setUser(res.data.data))
      .catch(() => localStorage.removeItem('token'))
      .finally(() => setIsLoading(false))
  }, [])

  const login = async (employeeId: string, password: string) => {
    const res = await api.post('/auth/login', { employee_id: employeeId, password })
    const { access_token, user } = res.data.data
    localStorage.setItem('token', access_token)
    setUser(user)
  }

  const logout = () => {
    localStorage.removeItem('token')
    setUser(null)
    queryClient.clear()
  }

  return (
    <AuthContext.Provider value={{ user, login, logout, isLoading }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be inside AuthProvider')
  return ctx
}
