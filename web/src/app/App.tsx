import { Component, lazy, Suspense, useEffect, type ReactNode } from "react";
import {
  Link,
  Navigate,
  NavLink,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import {
  Compass,
  Heart,
  History,
  LogOut,
  MapPin,
  Settings2,
  UserRound,
} from "lucide-react";
import { useSession } from "./session";
import { identity } from "../modules/identity/api";
import { Empty, ErrorBox, Loading } from "../shared/ui";
const Explore = lazy(() => import("../pages/Explore"));
const Place = lazy(() => import("../pages/Place"));
const Auth = lazy(() => import("../pages/Auth"));
const Profile = lazy(() => import("../pages/Profile"));
const Favorites = lazy(() => import("../pages/Favorites"));
const HistoryPage = lazy(() => import("../pages/History"));
const AdminPlaces = lazy(() => import("../pages/AdminPlaces"));
const PlaceEditor = lazy(() => import("../pages/PlaceEditor"));

function Guard({
  children,
  admin = false,
}: {
  children: ReactNode;
  admin?: boolean;
}) {
  const session = useSession();
  const location = useLocation();
  if (session.loading) return <Loading />;
  if (session.error)
    return <ErrorBox error={session.error} retry={session.reload} />;
  if (!session.user)
    return (
      <Navigate
        to={
          "/login?returnTo=" +
          encodeURIComponent(location.pathname + location.search)
        }
        replace
      />
    );
  if (admin && session.user.role !== "admin")
    return (
      <Empty title="Доступ только для администратора">
        <Link to="/explore">Вернуться к местам</Link>
      </Empty>
    );
  return children;
}
class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? (
      <Empty title="Не удалось открыть страницу">
        <p>Обновите страницу, чтобы попробовать снова.</p>
        <button
          className="button primary"
          onClick={() => window.location.reload()}
        >
          Обновить
        </button>
      </Empty>
    ) : (
      this.props.children
    );
  }
}
export default function App() {
  const session = useSession();
  const location = useLocation();
  const navigate = useNavigate();
  const logout = useMutation({
    mutationFn: identity.logout,
    onSuccess: () => {
      session.accept(null);
      navigate("/explore");
    },
  });
  useEffect(() => {
    window.scrollTo(0, 0);
    document.title = "GeoAssist — места Астаны";
  }, [location.pathname]);
  return (
    <div className="app">
      <a href="#main" className="skip-link">
        К содержимому
      </a>
      <header className="site-header">
        <div className="header-inner">
          <Link
            to="/explore"
            className="brand"
            aria-label="GeoAssist — главная"
          >
            <span className="brand-mark">
              <Compass size={25} />
            </span>
            <span>
              Geo<span className="brand-light">Assist</span>
              <small>АСТАНА</small>
            </span>
          </Link>
          <nav className="main-nav" aria-label="Главное меню">
            <NavLink to="/explore">
              <MapPin size={17} />
              <span>Места</span>
            </NavLink>
            {session.user && (
              <>
                <NavLink to="/favorites">
                  <Heart size={17} />
                  <span>Избранное</span>
                </NavLink>
                <NavLink to="/history">
                  <History size={17} />
                  <span>История</span>
                </NavLink>
              </>
            )}
            {session.user?.role === "admin" && (
              <NavLink to="/admin/places">
                <Settings2 size={17} />
                <span>Управление</span>
              </NavLink>
            )}
          </nav>
          <div className="account-nav">
            {session.loading ? (
              <span className="muted">Загружаем…</span>
            ) : session.error ? (
              <button className="text-button" onClick={session.reload}>
                Повторить вход
              </button>
            ) : session.user ? (
              <>
                <NavLink to="/profile" className="profile-link">
                  <UserRound size={18} />
                  <span>{session.user.display_name}</span>
                </NavLink>
                <button
                  className="icon-button"
                  aria-label="Выйти"
                  title="Выйти"
                  disabled={logout.isPending}
                  onClick={() => logout.mutate()}
                >
                  <LogOut size={18} />
                </button>
              </>
            ) : (
              <Link
                to={
                  "/login?returnTo=" +
                  encodeURIComponent(location.pathname + location.search)
                }
                className="button primary small"
              >
                Войти <UserRound size={16} />
              </Link>
            )}
          </div>
        </div>
      </header>
      <main id="main" className="main-content">
        <ErrorBox error={logout.error} />
        <ErrorBoundary key={location.pathname}>
          <Suspense fallback={<Loading />}>
            <Routes>
              <Route path="/" element={<Navigate to="/explore" replace />} />
              <Route path="/explore" element={<Explore />} />
              <Route
                path="/places/:id"
                element={<Place key={location.key} />}
              />
              <Route path="/login" element={<Auth key="login" />} />
              <Route
                path="/register"
                element={<Auth key="register" register />}
              />
              <Route
                path="/profile"
                element={
                  <Guard>
                    <Profile key={session.user?.id} />
                  </Guard>
                }
              />
              <Route
                path="/favorites"
                element={
                  <Guard>
                    <Favorites />
                  </Guard>
                }
              />
              <Route
                path="/history"
                element={
                  <Guard>
                    <HistoryPage />
                  </Guard>
                }
              />
              <Route
                path="/admin/places"
                element={
                  <Guard admin>
                    <AdminPlaces />
                  </Guard>
                }
              />
              <Route
                path="/admin/places/new"
                element={
                  <Guard admin>
                    <PlaceEditor />
                  </Guard>
                }
              />
              <Route
                path="/admin/places/:id/edit"
                element={
                  <Guard admin>
                    <PlaceEditor />
                  </Guard>
                }
              />
              <Route
                path="*"
                element={
                  <Empty title="Такой страницы нет">
                    <Link className="button primary" to="/explore">
                      Найти места
                    </Link>
                  </Empty>
                }
              />
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </main>
      <footer className="site-footer">
        <span>
          <Compass size={16} /> GeoAssist
        </span>
        <p>Ближе к городу. Ближе к своим местам.</p>
        <span>Астана, Казахстан</span>
      </footer>
    </div>
  );
}
