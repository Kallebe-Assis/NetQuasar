/** Logo NetQuasar na tela de login com animação suave. */
export function LoginBrandLogo({ showTitle = true }: { showTitle?: boolean }) {
  return (
    <div className="login-brand" aria-hidden={false}>
      <img
        className="login-brand__logo"
        src="/Logo-NetQuasar%20II.png"
        alt="NetQuasar"
        width={160}
        height={160}
        decoding="async"
      />
      {showTitle ? <p className="login-brand__title">NetQuasar</p> : null}
    </div>
  );
}
