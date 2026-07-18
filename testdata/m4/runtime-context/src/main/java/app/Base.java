package app;

public abstract class Base implements Contract {
    @Override
    public final void run() {
        hook();
    }

    protected abstract void hook();
}
