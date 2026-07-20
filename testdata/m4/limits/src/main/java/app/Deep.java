package app;

public class Deep {
    public void a() { b(); }
    private void b() { c(); }
    private void c() { d(); }
    private void d() { e(); }
    private void e() { f(); }
    private void f() {}
}
