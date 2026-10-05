/** El historial vive solo en estado de componente (decision B del diseno): recargar lo borra. */
export default function AvisoNoSeGuarda() {
  return <p className="agente-no-se-guarda">Esta conversación no se guarda.</p>;
}
