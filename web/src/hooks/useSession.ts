import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Me, type SystemStatus } from '@/api'

/**
 * The session and instance state the whole shell reads.
 *
 * Everything here is a reflection of server state. Nothing about being logged
 * in, and nothing about the sudo window, is decided in the browser (threat
 * T-01-33); this hook only asks and renders the answer.
 */

export const SESSION_QUERY_KEY = ['auth', 'me'] as const
export const STATUS_QUERY_KEY = ['system', 'status'] as const

export function useSystemStatus() {
  return useQuery<SystemStatus>({
    queryKey: STATUS_QUERY_KEY,
    queryFn: api.status,
    // The chain verdict and setup_required are cheap and worth being current.
    refetchInterval: 60_000,
    retry: 1,
  })
}

export interface Session {
  me: Me | undefined
  /** True once we know there is no operator account yet (D-01). */
  setupRequired: boolean
  /** True while either the status or the identity request is still in flight. */
  loading: boolean
  /** True when the server confirmed an authenticated session. */
  authenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  loggingOut: boolean
}

export function useSession(): Session {
  const queryClient = useQueryClient()
  const status = useSystemStatus()
  const setupRequired = status.data?.setup_required === true

  const me = useQuery<Me>({
    queryKey: SESSION_QUERY_KEY,
    queryFn: api.me,
    // Asking who we are before we know an account exists produces a
    // guaranteed 401 and a pointless audit-visible request.
    enabled: status.isSuccess && !setupRequired,
    retry: false,
  })

  const login = useMutation({
    mutationFn: (credentials: { username: string; password: string }) =>
      api.login(credentials.username, credentials.password),
    onSuccess: async () => {
      // The session id rotates on login, so everything read before it was read
      // by a session that no longer exists.
      await queryClient.invalidateQueries()
    },
  })

  const logout = useMutation({
    mutationFn: api.logout,
    onSettled: async () => {
      queryClient.setQueryData(SESSION_QUERY_KEY, undefined)
      await queryClient.invalidateQueries()
    },
  })

  return {
    me: me.data,
    setupRequired,
    loading: status.isPending || (me.isPending && me.fetchStatus !== 'idle'),
    authenticated: me.isSuccess,
    login: async (username: string, password: string) => {
      await login.mutateAsync({ username, password })
    },
    logout: async () => {
      await logout.mutateAsync()
    },
    loggingOut: logout.isPending,
  }
}
