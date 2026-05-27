package main
import ("context";"fmt";"time";"github.com/netquasar/netquasar/quasar_backend/internal/probing")
func main() {
 ctx,_:=context.WithTimeout(context.Background(),3*time.Minute)
 for _, oid := range []string{"1.3.6.1.2.1.1.2.0","1.3.6.1.4.1.13464","1.3.6.1.4.1.13464.1.11.3.1","1.3.6.1.4.1.37950.1.1.6.1.1"} {
  v,t,e:=probing.SNMPWalk(ctx, probing.SNMPWalkParams{Host:"10.255.150.2",Community:"VERIFICA",RootOID:oid,Version:"2c",Timeout:45*time.Second,MaxRows:3000})
  fmt.Printf("%s rows=%d trunc=%v err=%q\n", oid, len(v), t, e)
  for i,x:=range v { if i>=3 { break }; fmt.Printf("  %s = %s\n", x.OID, trunc(x.Value,50)) }
 }
}
func trunc(s string,n int) string { if len(s)<=n { return s }; return s[:n]+"..." }
