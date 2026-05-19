import { useEffect, useState } from 'react'
import { AuthContext } from './AuthContext'
import type { User } from './AuthContext'
import api from '../api/client'
import { useQueryClient } from '@tanstack/react-query'

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const queryClient = useQueryClient()

  useEffect(() => {
    const initAuth = async () => {
      const token = localStorage.getItem('token')

      if (!token) {
        setIsLoading(false)
        return
      }

      try {
        const res = await api.get('/auth/me')
        setUser(res.data.data)
      } catch {
        localStorage.removeItem('token')
      } finally {
        setIsLoading(false)
      }
    }

    initAuth()
  }, [])

  const login = async (employeeId: string, password: string) => {
    const res = await api.post('/auth/login', {
      employee_id: employeeId,
      password,
    })

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